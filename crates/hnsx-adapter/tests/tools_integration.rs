//! Tool-layer integration test for the genai adapter.
//!
//! - Mocks an OpenAI-compatible chat endpoint with `wiremock`.
//! - The first chat response asks to call an HTTP tool.
//! - The adapter executes the HTTP tool against the same mock server.
//! - The second chat response returns the final text, which is yielded to the caller.

use futures::StreamExt;
use genai::ServiceTarget;
use genai::resolver::{AuthData, Endpoint, ServiceTargetResolver};
use serde_json::json;
use wiremock::matchers::{method, path};
use wiremock::{Match, Mock, MockServer, Request, ResponseTemplate};

struct BodyContains(&'static str);
impl Match for BodyContains {
    fn matches(&self, request: &Request) -> bool {
        String::from_utf8_lossy(&request.body).contains(self.0)
    }
}

struct BodyNotContains(&'static str);
impl Match for BodyNotContains {
    fn matches(&self, request: &Request) -> bool {
        !String::from_utf8_lossy(&request.body).contains(self.0)
    }
}

use hnsx_adapter::genai::{GenaiAgentFactory, genai_model_name};
use hnsx_core::agent::{
    AdapterConfig, AgentSpec, InvokeContext, ModelRef, PromptTemplate, Provider, ToolKind, ToolRef,
};
use hnsx_core::agent_factory::AgentFactory;
use hnsx_core::chunk::Chunk;

fn spec_with_tool(endpoint: String) -> AgentSpec {
    AgentSpec {
        id: "quoter".into(),
        description: "x".into(),
        model: ModelRef {
            provider: Provider::Openai,
            model: "gpt-4o-mini".into(),
            endpoint: Some(endpoint),
        },
        adapter: AdapterConfig {
            timeout_seconds: None,
            extra: json!({}),
        },
        tools: vec![ToolRef {
            kind: ToolKind::Http,
            name: "yahoo_quote".into(),
            config: json!({
                "base_url": "http://placeholder.will-be-overridden-by-mock",
                "description": "Fetch a stock quote",
            }),
        }],
        prompt: PromptTemplate {
            template: "You are the quoter.".into(),
            variables: json!({}),
        },
        sandbox: None,
        memory_window: None,
    }
}

fn openai_client(base_url: String) -> genai::Client {
    genai::Client::builder()
        .with_service_target_resolver(ServiceTargetResolver::from_resolver_fn(move |mut target: ServiceTarget| {
            target.endpoint = Endpoint::from_owned(base_url.clone());
            target.auth = AuthData::Key("sk-test".into());
            Ok(target)
        }))
        .build()
}

#[tokio::test(flavor = "multi_thread")]
async fn agent_runs_http_tool_and_returns_final_answer() {
    let server = MockServer::start().await;
    let base_url = server.uri();

    // The HTTP tool will hit this endpoint.
    Mock::given(method("GET"))
        .and(path("/quote/AAPL"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({"price": 150.25})))
        .expect(1)
        .mount(&server)
        .await;

    // First chat request asks to call the HTTP tool.
    let tool_call_sse = r#"data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"yahoo_quote","arguments":"{\"path\":\"/quote/AAPL\"}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

"#;
    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .and(BodyContains(r#""role":"user""#))
        .and(BodyNotContains(r#""role":"tool""#))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(tool_call_sse),
        )
        .expect(1)
        .mount(&server)
        .await;

    // Second chat request (after tool result) returns final text.
    let final_sse = r#"data: {"choices":[{"delta":{"content":"Analysis complete."}}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]

"#;
    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .and(BodyContains(r#""role":"tool""#))
        .respond_with(
            ResponseTemplate::new(200)
                .insert_header("content-type", "text/event-stream")
                .set_body_string(final_sse),
        )
        .expect(1)
        .mount(&server)
        .await;

    let mut spec = spec_with_tool(base_url.clone());
    // Point the HTTP tool at the mock server as well.
    spec.tools[0].config["base_url"] = json!(base_url);

    let factory = GenaiAgentFactory::with_client(openai_client(base_url));
    let agent = factory.create(&spec).expect("create agent");

    let ctx = InvokeContext {
        session_id: "s1".into(),
        domain_id: "financial-analysis".into(),
        agent_id: "quoter".into(),
    };
    let stream = agent
        .invoke(json!({"ticker": "AAPL"}), ctx)
        .await
        .expect("invoke");

    let chunks: Vec<Chunk> = stream.collect().await;
    let text: String = chunks
        .iter()
        .filter_map(|c| match c {
            Chunk::Text(t) => Some(t.clone()),
            _ => None,
        })
        .collect();

    assert!(
        text.contains("Analysis complete."),
        "expected final answer, got: {text}"
    );
    assert_eq!(genai_model_name(&spec).unwrap(), "openai::gpt-4o-mini");
}
