//! HTTP tool: GET / POST / PUT / DELETE / PATCH with flexible auth, query
//! parameters, retries, and structured error reporting.

use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use serde::Deserialize;
use serde_json::{json, Value};

use hnsx_core::agent::ToolKind;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::Tool;

const DEFAULT_TIMEOUT_MS: u64 = 30_000;
const DEFAULT_RETRIES: u32 = 0;

/// HTTP authentication variants.
#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum HttpAuth {
    /// `Authorization: Bearer <token>`.
    Bearer { token: String },
    /// Custom header name and value, e.g. `x-api-key`.
    Header { name: String, value: String },
    /// Query parameter key/value, e.g. `api_key=secret`.
    Query { name: String, value: String },
}

/// Tool config. Persisted as part of `ToolRef.config` in `AgentSpec`.
#[derive(Debug, Clone, Deserialize)]
pub struct HttpConfig {
    /// Optional default base URL. Each `invoke` call can override via `args.url`.
    #[serde(default)]
    pub base_url: Option<String>,
    /// Optional default auth. Each call can override via `args.auth`.
    #[serde(default)]
    pub default_auth: Option<HttpAuth>,
    /// Optional default Bearer token shorthand. Each call can override via
    /// `args.token` or `args.auth`.
    #[serde(default)]
    pub default_token: Option<String>,
    /// Default request timeout in ms. Defaults to 30s.
    #[serde(default)]
    pub timeout_ms: Option<u64>,
    /// Number of retries on transient failures (5xx or network error). Default 0.
    #[serde(default)]
    pub retries: Option<u32>,
}

/// Per-call args. Recognised fields:
///   method: "GET" | "POST" | "PUT" | "DELETE" | "PATCH"   (default: "GET")
///   path:   "/foo"                              (joined to base_url if set)
///   url:    full URL                             (overrides base_url + path)
///   auth:   { "type": "bearer", "token": "..." }
///           { "type": "header", "name": "x-api-key", "value": "..." }
///           { "type": "query", "name": "api_key", "value": "..." }
///   token:  bearer token shorthand                 (overrides default_token)
///   query:  { "key": "value" }                    (merged with url query)
///   body:   any JSON value                        (POST/PUT/PATCH)
///   headers: { "x-custom": "value" }              (merged)
pub struct HttpTool {
    name: String,
    config: Value,
    client: reqwest::Client,
    retries: u32,
}

impl HttpTool {
    pub fn new(name: impl Into<String>, config: Value) -> Result<Arc<Self>> {
        let cfg: HttpConfig = serde_json::from_value(config.clone())
            .map_err(|e| Error::Adapter(format!("HttpTool config: {e}")))?;
        let timeout = Duration::from_millis(cfg.timeout_ms.unwrap_or(DEFAULT_TIMEOUT_MS));
        let client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .map_err(|e| Error::Adapter(format!("HttpTool client: {e}")))?;
        Ok(Arc::new(Self {
            name: name.into(),
            config,
            client,
            retries: cfg.retries.unwrap_or(DEFAULT_RETRIES),
        }))
    }
}

#[async_trait]
impl Tool for HttpTool {
    fn name(&self) -> &str {
        &self.name
    }
    fn kind(&self) -> ToolKind {
        ToolKind::Http
    }
    fn config(&self) -> &Value {
        &self.config
    }

    async fn invoke(&self, args: Value) -> Result<Value> {
        let cfg: HttpConfig = serde_json::from_value(self.config.clone())
            .map_err(|e| Error::Adapter(format!("HttpTool config: {e}")))?;

        let method = args
            .get("method")
            .and_then(Value::as_str)
            .unwrap_or("GET")
            .to_ascii_uppercase();

        let mut url: reqwest::Url = match args.get("url").and_then(Value::as_str) {
            Some(u) => u.parse().map_err(|e| Error::Adapter(format!("HttpTool bad url: {e}")))?,
            None => {
                let path = args
                    .get("path")
                    .and_then(Value::as_str)
                    .ok_or_else(|| Error::Adapter("HttpTool: either `url` or `path` is required".into()))?;
                let base = cfg
                    .base_url
                    .as_deref()
                    .ok_or_else(|| Error::Adapter("HttpTool: `path` given but no `base_url` in config".into()))?;
                let base = base.trim_end_matches('/');
                format!("{base}{path}")
                    .parse()
                    .map_err(|e| Error::Adapter(format!("HttpTool bad base_url/path: {e}")))?
            }
        };

        // Merge query params from args and auth.
        let mut query: Vec<(String, String)> = Vec::new();
        if let Some(q) = args.get("query").and_then(Value::as_object) {
            for (k, v) in q {
                query.push((k.clone(), json_value_to_query(v)));
            }
        }

        let auth = resolve_auth(&args, &cfg)?;
        if let Some(HttpAuth::Query { ref name, ref value }) = auth {
            query.push((name.clone(), value.clone()));
        }
        if !query.is_empty() {
            url.query_pairs_mut().extend_pairs(query);
        }

        let method = method
            .parse::<reqwest::Method>()
            .map_err(|e| Error::Adapter(format!("HttpTool: bad method {method}: {e}")))?;

        let mut attempt = 0;
        loop {
            let mut req = self.client.request(method.clone(), url.clone());

            // Apply auth.
            match &auth {
                Some(HttpAuth::Bearer { token }) => {
                    req = req.bearer_auth(token);
                }
                Some(HttpAuth::Header { name, value }) => {
                    req = req.header(name, value);
                }
                Some(HttpAuth::Query { .. }) | None => {}
            }

            // Apply custom headers.
            if let Some(headers) = args.get("headers").and_then(Value::as_object) {
                for (k, v) in headers {
                    if let Some(s) = v.as_str() {
                        req = req.header(k, s);
                    }
                }
            }

            // Apply body.
            if matches!(method.as_str(), "POST" | "PUT" | "PATCH") {
                if let Some(body) = args.get("body") {
                    req = req.json(body);
                }
            }

            match req.send().await {
                Ok(resp) => {
                    let status = resp.status();
                    let body_text = resp
                        .text()
                        .await
                        .map_err(|e| Error::Adapter(format!("HttpTool read body: {e}")))?;
                    let body_value: Value = serde_json::from_str(&body_text)
                        .unwrap_or(Value::String(body_text.clone()));

                    return Ok(json!({
                        "status": status.as_u16(),
                        "ok": status.is_success(),
                        "body": body_value,
                    }));
                }
                Err(e) => {
                    let msg = format!("HttpTool send: {e}");
                    if attempt >= self.retries || !is_transient(&e) {
                        return Err(Error::Adapter(msg));
                    }
                    attempt += 1;
                    tokio::time::sleep(Duration::from_millis(500 * u64::from(attempt))).await;
                }
            }
        }
    }
}

fn resolve_auth(args: &Value, cfg: &HttpConfig) -> Result<Option<HttpAuth>> {
    if let Some(auth) = args.get("auth") {
        return serde_json::from_value(auth.clone())
            .map_err(|e| Error::Adapter(format!("HttpTool auth: {e}")))
            .map(Some);
    }
    if let Some(token) = args.get("token").and_then(Value::as_str) {
        return Ok(Some(HttpAuth::Bearer { token: token.into() }));
    }
    if let Some(token) = cfg.default_token.as_deref() {
        return Ok(Some(HttpAuth::Bearer { token: token.into() }));
    }
    Ok(cfg.default_auth.clone())
}

fn json_value_to_query(v: &Value) -> String {
    match v {
        Value::String(s) => s.clone(),
        Value::Number(n) => n.to_string(),
        Value::Bool(b) => b.to_string(),
        other => other.to_string(),
    }
}

fn is_transient(e: &reqwest::Error) -> bool {
    e.is_timeout() || e.is_connect() || e.is_request()
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{header, method, path, query_param};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test(flavor = "multi_thread")]
    async fn get_with_bearer_token() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/v1/ping"))
            .and(header("Authorization", "Bearer sk-test"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({"ok": true})))
            .expect(1)
            .mount(&server)
            .await;

        let tool = HttpTool::new(
            "ping",
            json!({"base_url": server.uri(), "default_token": "sk-test"}),
        )
        .expect("build");

        let out = tool
            .invoke(json!({"method": "GET", "path": "/v1/ping"}))
            .await
            .expect("invoke");
        assert_eq!(out["status"], 200);
        assert_eq!(out["ok"], true);
        assert_eq!(out["body"]["ok"], true);
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn post_with_body() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/echo"))
            .respond_with(ResponseTemplate::new(201).set_body_json(json!({"received": true})))
            .mount(&server)
            .await;

        let tool = HttpTool::new("echo", json!({"base_url": server.uri()})).expect("build");
        let out = tool
            .invoke(json!({
                "method": "POST",
                "path": "/v1/echo",
                "body": {"hello": "world"}
            }))
            .await
            .expect("invoke");
        assert_eq!(out["status"], 201);
        assert_eq!(out["body"]["received"], true);
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn absolute_url_overrides_base() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/elsewhere"))
            .respond_with(ResponseTemplate::new(200).set_body_string("ok"))
            .mount(&server)
            .await;

        let tool = HttpTool::new("t", json!({"base_url": "http://nope.invalid"})).expect("build");
        let out = tool
            .invoke(json!({"url": format!("{}/elsewhere", server.uri())}))
            .await
            .expect("invoke");
        assert_eq!(out["ok"], true);
        assert_eq!(out["body"], "ok");
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn custom_header_auth() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/api"))
            .and(header("x-api-key", "secret"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&server)
            .await;

        let tool = HttpTool::new(
            "api",
            json!({
                "base_url": server.uri(),
                "default_auth": {"type": "header", "name": "x-api-key", "value": "secret"}
            }),
        )
        .expect("build");

        let out = tool.invoke(json!({"path": "/api"})).await.expect("invoke");
        assert_eq!(out["ok"], true);
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn query_params_from_args_and_auth() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/search"))
            .and(query_param("q", "hello"))
            .and(query_param("api_key", "secret"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({"hits": 1})))
            .mount(&server)
            .await;

        let tool = HttpTool::new(
            "search",
            json!({
                "base_url": server.uri(),
                "default_auth": {"type": "query", "name": "api_key", "value": "secret"}
            }),
        )
        .expect("build");

        let out = tool
            .invoke(json!({"path": "/search", "query": {"q": "hello"}}))
            .await
            .expect("invoke");
        assert_eq!(out["body"]["hits"], 1);
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn retries_on_network_failure() {
        // Connect to a closed port to force a connection error.
        let tool = HttpTool::new(
            "flaky",
            json!({"base_url": "http://127.0.0.1:1", "retries": 1}),
        )
        .expect("build");

        let err = tool.invoke(json!({"path": "/flaky"})).await.unwrap_err();
        let msg = format!("{err}");
        assert!(msg.contains("HttpTool send"), "msg={msg}");
    }
}
