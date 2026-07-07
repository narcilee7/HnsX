import { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { api, TraceRecord, Session } from '../api';

export default function Traces() {
  const { domainId } = useParams<{ domainId: string }>();
  const [traces, setTraces] = useState<TraceRecord[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [selectedSession, setSelectedSession] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!domainId) return;
    Promise.all([
      api.listSessions(domainId),
      api.listTraces(domainId),
    ])
      .then(([s, t]) => {
        setSessions(s);
        setTraces(t);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [domainId]);

  useEffect(() => {
    if (!domainId) return;
    setLoading(true);
    api
      .listTraces(domainId, selectedSession || undefined)
      .then(setTraces)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [domainId, selectedSession]);

  if (loading) return <p>Loading traces...</p>;
  if (error) return <p className="error">{error}</p>;

  return (
    <div>
      <h2>Traces for {domainId}</h2>
      <div className="actions" style={{ marginBottom: '1rem' }}>
        <Link to="/">← Back</Link>
        <Link to={`/domains/${domainId}`}>Instances</Link>
        <Link to={`/metrics/${domainId}`}>Metrics</Link>
      </div>

      <div className="card" style={{ marginBottom: '1rem' }}>
        <label htmlFor="session">Session: </label>
        <select
          id="session"
          value={selectedSession}
          onChange={(e) => setSelectedSession(e.target.value)}
          style={{ marginLeft: '0.5rem' }}
        >
          <option value="">All sessions ({sessions.length})</option>
          {sessions.map((s) => (
            <option key={s.session_id} value={s.session_id}>
              {s.session_id} · {s.step_count} steps · {new Date(s.started_at_ms).toLocaleString()}
            </option>
          ))}
        </select>
      </div>

      {traces.length === 0 ? (
        <p>No traces found.</p>
      ) : (
        traces.map((t) => (
          <div key={`${t.session_id}-${t.step_id}-${t.started_at_ms}`} className="card">
            <h3>
              {t.step_id}{' '}
              <span style={{ color: 'var(--muted)', fontSize: '0.85rem' }}>({t.agent_id})</span>
            </h3>
            <p>
              Session: {t.session_id} · Duration: {t.duration_ms}ms · Started:{' '}
              {new Date(t.started_at_ms).toLocaleString()}
            </p>
            <p>Input:</p>
            <pre>{t.input}</pre>
            <p>Output:</p>
            <pre>{t.output}</pre>
          </div>
        ))
      )}
    </div>
  );
}
