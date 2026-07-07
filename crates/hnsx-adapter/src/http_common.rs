//! Shared HTTP adapter utilities: SSE parsing, token/cost math, and error
//! classification for native OpenAI/Anthropic/Ollama/Custom adapters.

use async_stream::stream;
use bytes::Bytes;
use futures::stream::{BoxStream, Stream, StreamExt};
use serde_json::Value;

use hnsx_core::error::{Error, Result};

/// A single Server-Sent Events event.
#[derive(Debug, Clone, Default)]
pub struct SSEEvent {
    pub event: Option<String>,
    pub data: String,
}

impl SSEEvent {
    pub fn json(&self) -> Result<Value> {
        if self.data.is_empty() || self.data == "[DONE]" {
            return Ok(Value::Null);
        }
        serde_json::from_str(&self.data).map_err(Error::Json)
    }
}

/// Turn a stream of HTTP body chunks into a stream of parsed SSE events.
pub fn parse_sse_stream<S, E>(source: S) -> BoxStream<'static, Result<SSEEvent>>
where
    S: Stream<Item = std::result::Result<Bytes, E>> + Send + 'static,
    E: std::error::Error + Send + Sync + 'static,
{
    let mut source = Box::pin(source);
    let mut accumulator = String::new();

    Box::pin(stream! {
        loop {
            match source.next().await {
                Some(Ok(bytes)) => {
                    accumulator.push_str(&String::from_utf8_lossy(&bytes));
                    while let Some((event, rest)) = pop_event(&accumulator) {
                        accumulator = rest;
                        yield Ok(event);
                    }
                }
                Some(Err(e)) => {
                    yield Err(Error::Adapter(format!("SSE stream error: {e}")));
                    return;
                }
                None => {
                    // Drain any remaining event.
                    while let Some((event, rest)) = pop_event(&accumulator) {
                        accumulator = rest;
                        yield Ok(event);
                    }
                    return;
                }
            }
        }
    })
}

/// Try to pop one SSE event from the front of the buffer. Returns the event and
/// the remaining buffer after the terminating blank line. Empty events caused by
/// keep-alive blank lines are consumed and skipped.
fn pop_event(buffer: &str) -> Option<(SSEEvent, String)> {
    let (split, sep_len) = buffer
        .find("\r\n\r\n")
        .map(|p| (p, 4))
        .or_else(|| buffer.find("\n\n").map(|p| (p, 2)))?;

    let event_text = &buffer[..split];
    let rest = buffer[split + sep_len..].to_string();

    let mut event = SSEEvent::default();
    for line in event_text.lines() {
        if let Some(data) = line.strip_prefix("data:") {
            let data = data.trim_start();
            if !event.data.is_empty() {
                event.data.push('\n');
            }
            event.data.push_str(data);
        } else if let Some(ev) = line.strip_prefix("event:") {
            event.event = Some(ev.trim().to_string());
        }
    }

    if event.data.is_empty() && event.event.is_none() {
        // Keep-alive or empty event; recurse on the rest.
        return pop_event(&rest);
    }

    Some((event, rest))
}

/// Cost in USD for OpenAI models (per 1M tokens). Unknown models report 0.
pub fn openai_cost(model: &str, prompt_tokens: u64, completion_tokens: u64) -> f64 {
    let (prompt_per_1m, completion_per_1m) = match model {
        m if m.contains("gpt-4o-mini") => (0.15, 0.60),
        m if m.contains("gpt-4o") => (2.50, 10.00),
        m if m.contains("gpt-4-turbo") => (10.00, 30.00),
        m if m.contains("gpt-4") => (30.00, 60.00),
        m if m.contains("gpt-3.5-turbo") => (0.50, 1.50),
        _ => (0.0, 0.0),
    };
    (prompt_tokens as f64 * prompt_per_1m + completion_tokens as f64 * completion_per_1m) / 1_000_000.0
}

/// Cost in USD for Anthropic models (per 1M tokens). Unknown models report 0.
pub fn anthropic_cost(model: &str, prompt_tokens: u64, completion_tokens: u64) -> f64 {
    let (prompt_per_1m, completion_per_1m) = match model {
        m if m.contains("claude-3-5-sonnet") => (3.00, 15.00),
        m if m.contains("claude-3-sonnet") => (3.00, 15.00),
        m if m.contains("claude-3-opus") => (15.00, 75.00),
        m if m.contains("claude-3-haiku") => (0.25, 1.25),
        m if m.contains("claude-haiku") => (0.25, 1.25),
        _ => (0.0, 0.0),
    };
    (prompt_tokens as f64 * prompt_per_1m + completion_tokens as f64 * completion_per_1m) / 1_000_000.0
}

/// Estimate tokens when the API does not report usage. This is intentionally
/// rough: ~4 characters per token.
pub fn estimate_tokens(text: &str) -> u64 {
    (text.chars().count() as u64 / 4).max(1)
}

/// Classify an HTTP error status into an `Error::Adapter` with a clear message.
pub fn classify_http_error(status: reqwest::StatusCode, body: &str) -> Error {
    let short = body.chars().take(512).collect::<String>();
    match status.as_u16() {
        401 | 403 => Error::Adapter(format!("authentication failed ({status}): {short}")),
        429 => Error::Adapter(format!("rate limit ({status}): {short}")),
        400..=499 => Error::Adapter(format!("bad request ({status}): {short}")),
        500..=599 => Error::Adapter(format!("upstream server error ({status}): {short}")),
        _ => Error::Adapter(format!("upstream error ({status}): {short}")),
    }
}

/// Build an Authorization header value from an API key.
pub fn bearer(api_key: &str) -> String {
    format!("Bearer {api_key}")
}

/// Extract the first non-empty string from a JSON value (useful for prompts).
pub fn value_to_string(value: &Value) -> String {
    match value {
        Value::String(s) => s.clone(),
        Value::Null => String::new(),
        other => other.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use futures::StreamExt;

    fn sse_bytes(s: &str) -> impl Stream<Item = std::result::Result<Bytes, std::io::Error>> {
        let bytes = Bytes::from(s.to_string());
        futures::stream::iter(vec![Ok(bytes)])
    }

    #[tokio::test]
    async fn parses_simple_sse_event() {
        let stream = sse_bytes("data: {\"x\":1}\n\n");
        let events: Vec<_> = parse_sse_stream(stream)
            .filter_map(|r| async move { r.ok() })
            .collect()
            .await;
        assert_eq!(events.len(), 1);
        assert_eq!(events[0].data, "{\"x\":1}");
        assert_eq!(events[0].event, None);
    }

    #[tokio::test]
    async fn parses_event_name() {
        let stream = sse_bytes("event: message\ndata: hello\n\n");
        let events: Vec<_> = parse_sse_stream(stream)
            .filter_map(|r| async move { r.ok() })
            .collect()
            .await;
        assert_eq!(events.len(), 1);
        assert_eq!(events[0].event.as_deref(), Some("message"));
        assert_eq!(events[0].data, "hello");
    }

    #[tokio::test]
    async fn ignores_done_marker() {
        let stream = sse_bytes("data: [DONE]\n\n");
        let events: Vec<_> = parse_sse_stream(stream)
            .filter_map(|r| async move { r.ok() })
            .collect()
            .await;
        assert_eq!(events.len(), 1);
        assert!(events[0].json().unwrap().is_null());
    }

    #[test]
    fn openai_cost_known_models() {
        assert!(openai_cost("gpt-4o-mini", 1_000_000, 1_000_000) > 0.0);
        assert_eq!(openai_cost("gpt-4o-mini", 1_000_000, 0), 0.15);
        assert_eq!(openai_cost("unknown", 1_000_000, 1_000_000), 0.0);
    }

    #[test]
    fn anthropic_cost_known_models() {
        assert!(anthropic_cost("claude-3-sonnet", 1_000_000, 1_000_000) > 0.0);
        assert_eq!(anthropic_cost("unknown", 1_000_000, 1_000_000), 0.0);
    }

    #[test]
    fn estimate_tokens_positive() {
        assert!(estimate_tokens("hello world") >= 1);
    }
}
