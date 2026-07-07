//! HTTP tool: GET / POST / PUT / DELETE with Bearer auth and timeout.

use std::sync::Arc;

use async_trait::async_trait;
use serde::Deserialize;
use serde_json::{json, Value};

use hnsx_core::agent::ToolKind;
use hnsx_core::error::{Error, Result};
use hnsx_core::tool::Tool;

const DEFAULT_TIMEOUT_MS: u64 = 30_000;

/// Tool config. Persisted as part of `ToolRef.config` in `AgentSpec`.
#[derive(Debug, Clone, Deserialize)]
pub struct HttpConfig {
    /// Optional default base URL. Each `invoke` call can override via `args.url`.
    #[serde(default)]
    pub base_url: Option<String>,
    /// Optional default Bearer token. Each call can override via `args.token`.
    #[serde(default)]
    pub default_token: Option<String>,
    /// Default request timeout in ms. Defaults to 30s.
    #[serde(default)]
    pub timeout_ms: Option<u64>,
}

/// Per-call args. Recognised fields:
///   method: "GET" | "POST" | "PUT" | "DELETE"   (default: "GET")
///   path:   "/foo"                              (joined to base_url if set)
///   url:    full URL                             (overrides base_url + path)
///   token:  bearer token                          (overrides default)
///   body:   any JSON value                        (POST/PUT only)
pub struct HttpTool {
    name: String,
    config: Value,
    client: reqwest::Client,
}

impl HttpTool {
    pub fn new(name: impl Into<String>, config: Value) -> Result<Arc<Self>> {
        let cfg: HttpConfig = serde_json::from_value(config.clone())
            .map_err(|e| Error::Adapter(format!("HttpTool config: {e}")))?;
        let timeout = std::time::Duration::from_millis(cfg.timeout_ms.unwrap_or(DEFAULT_TIMEOUT_MS));
        let client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .map_err(|e| Error::Adapter(format!("HttpTool client: {e}")))?;
        Ok(Arc::new(Self {
            name: name.into(),
            config,
            client,
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

        let url = match args.get("url").and_then(Value::as_str) {
            Some(u) => u.to_string(),
            None => {
                let path = args
                    .get("path")
                    .and_then(Value::as_str)
                    .ok_or_else(|| Error::Adapter("HttpTool: either `url` or `path` is required".into()))?;
                let base = cfg
                    .base_url
                    .as_deref()
                    .ok_or_else(|| Error::Adapter("HttpTool: `path` given but no `base_url` in config".into()))?;
                format!("{}{}", base.trim_end_matches('/'), path)
            }
        };

        let token = args
            .get("token")
            .and_then(Value::as_str)
            .or(cfg.default_token.as_deref());

        let mut req = self
            .client
            .request(
                method
                    .parse::<reqwest::Method>()
                    .map_err(|e| Error::Adapter(format!("HttpTool: bad method {method}: {e}")))?,
                &url,
            );

        if let Some(t) = token {
            req = req.bearer_auth(t);
        }

        if matches!(method.as_str(), "POST" | "PUT" | "PATCH") {
            if let Some(body) = args.get("body") {
                req = req.json(body);
            }
        }

        let resp = req
            .send()
            .await
            .map_err(|e| Error::Adapter(format!("HttpTool send: {e}")))?;

        let status = resp.status();
        let body_text = resp
            .text()
            .await
            .map_err(|e| Error::Adapter(format!("HttpTool read body: {e}")))?;

        // Try to parse as JSON; fall back to text.
        let body_value: Value = serde_json::from_str(&body_text)
            .unwrap_or(Value::String(body_text.clone()));

        Ok(json!({
            "status": status.as_u16(),
            "ok": status.is_success(),
            "body": body_value,
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test(flavor = "multi_thread")]
    async fn get_with_bearer_token() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/v1/ping"))
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
}
