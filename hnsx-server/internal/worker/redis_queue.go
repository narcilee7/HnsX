package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSessionQueue persists pending sessions in Redis so multiple Control
// Plane instances can share the same scheduling queue.
//
// Key layout (all keys share the prefix):
//
//	{prefix}:pending  -> Redis List of session IDs (head = newest)
//	{prefix}:ids      -> Redis Set of queued session IDs (idempotency)
//	{prefix}:req:{id} -> Redis Hash with SessionRequest fields
//
// Operations use Lua scripts to keep matching + deletion atomic.
type RedisSessionQueue struct {
	rdb    *redis.Client
	prefix string
	ttl    time.Duration
}

// NewRedisSessionQueue constructs a queue backed by the supplied Redis client.
func NewRedisSessionQueue(rdb *redis.Client, prefix string) *RedisSessionQueue {
	if prefix == "" {
		prefix = "hnsx:queue"
	}
	return &RedisSessionQueue{
		rdb:    rdb,
		prefix: prefix,
		ttl:    24 * time.Hour,
	}
}

func (q *RedisSessionQueue) keyPending() string { return q.prefix + ":pending" }
func (q *RedisSessionQueue) keySet() string     { return q.prefix + ":ids" }
func (q *RedisSessionQueue) keyReq(id string) string {
	return q.prefix + ":req:" + id
}

// Enqueue adds a session to the Redis queue. The operation is idempotent:
// enqueuing the same SessionID twice is a no-op.
func (q *RedisSessionQueue) Enqueue(req *SessionRequest) {
	if req == nil || req.SessionID == "" {
		return
	}
	if req.EnqueuedAt.IsZero() {
		req.EnqueuedAt = time.Now().UTC()
	}
	ctx := context.Background()
	// Lua: only insert when the id is not already present.
	const script = `
		local setKey = KEYS[1]
		local pendingKey = KEYS[2]
		local reqKey = KEYS[3]
		local id = ARGV[1]
		if redis.call("SISMEMBER", setKey, id) == 1 then
			return 0
		end
		redis.call("SADD", setKey, id)
		redis.call("RPUSH", pendingKey, id)
		redis.call("HSET", reqKey,
			"session_id", id,
			"domain_id", ARGV[2],
			"domain_version", ARGV[3],
			"domain_spec_json", ARGV[4],
			"trigger_payload_json", ARGV[5],
			"trace_id", ARGV[6],
			"required_capabilities", ARGV[7],
			"enqueued_at", ARGV[8],
			"correlation_id", ARGV[9])
		redis.call("EXPIRE", reqKey, tonumber(ARGV[10]))
		return 1
	`
	caps := strings.Join(req.RequiredCapabilities, ",")
	_, _ = q.rdb.Eval(ctx, script,
		[]string{q.keySet(), q.keyPending(), q.keyReq(req.SessionID)},
		req.SessionID,
		req.DomainID,
		req.DomainVersion,
		req.DomainSpecJSON,
		req.TriggerPayloadJSON,
		req.TraceID,
		caps,
		fmt.Sprintf("%d", req.EnqueuedAt.UnixMilli()),
		req.CorrelationID,
		int64(q.ttl.Seconds()),
	).Result()
}

// Dequeue blocks until a session matching all required capabilities is
// available or the context is cancelled. It polls Redis at 100ms intervals.
func (q *RedisSessionQueue) Dequeue(ctx context.Context, required []string) (*SessionRequest, bool) {
	requiredStr := strings.Join(required, ",")
	const script = `
		local pendingKey = KEYS[1]
		local setKey = KEYS[2]
		local prefix = ARGV[1]
		local requiredStr = ARGV[2]

		local function matches(offeredStr, reqStr)
			if reqStr == "" then return true end
			local required = {}
			for cap in string.gmatch(reqStr, "([^,]+)") do
				required[cap] = true
			end
			local have = {}
			for cap in string.gmatch(offeredStr, "([^,]+)") do
				have[cap] = true
			end
			for cap, _ in pairs(required) do
				if not have[cap] then return false end
			end
			return true
		end

		local ids = redis.call("LRANGE", pendingKey, 0, -1)
		for _, id in ipairs(ids) do
			local reqKey = prefix .. ":req:" .. id
			local fields = redis.call("HGETALL", reqKey)
			if #fields > 0 then
				local m = {}
				for i = 1, #fields, 2 do
					m[fields[i]] = fields[i+1]
				end
				if matches(m.required_capabilities or "", requiredStr) then
					redis.call("LREM", pendingKey, 0, id)
					redis.call("SREM", setKey, id)
					redis.call("DEL", reqKey)
					return fields
				end
			end
		end
		return nil
	`
	for {
		res, err := q.rdb.Eval(ctx, script,
			[]string{q.keyPending(), q.keySet()},
			q.prefix, requiredStr,
		).Result()
		if err != nil && err != redis.Nil {
			// Treat transient Redis errors as "no match" and retry.
		}
		if vals, ok := res.([]interface{}); ok && len(vals) > 0 {
			m := parseRedisHash(vals)
			if req := q.reqFromMap(m); req != nil {
				return req, true
			}
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Remove deletes a session from the queue.
func (q *RedisSessionQueue) Remove(id string) {
	if id == "" {
		return
	}
	ctx := context.Background()
	const script = `
		local pendingKey = KEYS[1]
		local setKey = KEYS[2]
		local reqKey = KEYS[3]
		local id = ARGV[1]
		redis.call("LREM", pendingKey, 0, id)
		redis.call("SREM", setKey, id)
		redis.call("DEL", reqKey)
		return 1
	`
	_, _ = q.rdb.Eval(ctx, script,
		[]string{q.keyPending(), q.keySet(), q.keyReq(id)},
		id,
	).Result()
}

// Len returns the number of pending sessions.
func (q *RedisSessionQueue) Len() int {
	ctx := context.Background()
	n, err := q.rdb.LLen(ctx, q.keyPending()).Result()
	if err != nil {
		return 0
	}
	return int(n)
}

// Recover bulk-loads pending sessions into Redis. Skips sessions already
// present in the idempotency set.
func (q *RedisSessionQueue) Recover(items []*SessionRequest) error {
	var filtered []*SessionRequest
	for _, req := range items {
		if req != nil && req.SessionID != "" {
			filtered = append(filtered, req)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	ctx := context.Background()
	const script = `
		local setKey = KEYS[1]
		local pendingKey = KEYS[2]
		local prefix = ARGV[1]
		local ttl = tonumber(ARGV[2])
		local count = tonumber(ARGV[3])
		local offset = 3
		for i = 1, count do
			local base = offset + (i - 1) * 9
			local id = ARGV[base + 1]
			if redis.call("SISMEMBER", setKey, id) == 0 then
				redis.call("SADD", setKey, id)
				redis.call("RPUSH", pendingKey, id)
				local reqKey = prefix .. ":req:" .. id
				redis.call("HSET", reqKey,
					"session_id", id,
					"domain_id", ARGV[base + 2],
					"domain_version", ARGV[base + 3],
					"domain_spec_json", ARGV[base + 4],
					"trigger_payload_json", ARGV[base + 5],
					"trace_id", ARGV[base + 6],
					"required_capabilities", ARGV[base + 7],
					"enqueued_at", ARGV[base + 8],
					"correlation_id", ARGV[base + 9])
				redis.call("EXPIRE", reqKey, ttl)
			end
		end
		return count
	`
	args := []any{q.prefix, int64(q.ttl.Seconds()), len(filtered)}
	for _, req := range filtered {
		enqueuedAt := req.EnqueuedAt
		if enqueuedAt.IsZero() {
			enqueuedAt = time.Now().UTC()
		}
		args = append(args,
			req.SessionID,
			req.DomainID,
			req.DomainVersion,
			req.DomainSpecJSON,
			req.TriggerPayloadJSON,
			req.TraceID,
			strings.Join(req.RequiredCapabilities, ","),
			fmt.Sprintf("%d", enqueuedAt.UnixMilli()),
			req.CorrelationID,
		)
	}
	_, err := q.rdb.Eval(ctx, script, []string{q.keySet(), q.keyPending()}, args...).Result()
	return err
}

func parseRedisHash(vals []interface{}) map[string]string {
	m := make(map[string]string, len(vals)/2)
	for i := 0; i+1 < len(vals); i += 2 {
		k, _ := vals[i].(string)
		v, _ := vals[i+1].(string)
		if k != "" {
			m[k] = v
		}
	}
	return m
}

func (q *RedisSessionQueue) reqFromMap(m map[string]string) *SessionRequest {
	id := m["session_id"]
	if id == "" {
		return nil
	}
	var enqueuedAt time.Time
	if ms, err := strconv.ParseInt(m["enqueued_at"], 10, 64); err == nil {
		enqueuedAt = time.UnixMilli(ms).UTC()
	}
	var caps []string
	if s := m["required_capabilities"]; s != "" {
		caps = strings.Split(s, ",")
	}
	return &SessionRequest{
		SessionID:            id,
		DomainID:             m["domain_id"],
		DomainVersion:        m["domain_version"],
		DomainSpecJSON:       m["domain_spec_json"],
		TriggerPayloadJSON:   m["trigger_payload_json"],
		TraceID:              m["trace_id"],
		RequiredCapabilities: caps,
		EnqueuedAt:           enqueuedAt,
		CorrelationID:        m["correlation_id"],
	}
}

var _ SessionQueue = (*RedisSessionQueue)(nil)
