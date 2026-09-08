// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::sync::Arc;

use axum::{extract::State, routing::post, Json, Router};
use serde_json::Value;
use tokio::sync::Mutex;
use uuid::Uuid;

use super::{
    provider_incarnation::PROVIDER_INCARNATION_ANNOTATION, sandboxes::SandboxService,
};
use crate::{
    cubemaster::CubeMasterClient,
    models::NewSandbox,
};

#[derive(Clone, Default)]
struct Capture {
    create_body: Arc<Mutex<Option<Value>>>,
}

async fn create_handler(
    State(capture): State<Capture>,
    Json(body): Json<Value>,
) -> Json<Value> {
    *capture.create_body.lock().await = Some(body);
    Json(serde_json::json!({
        "requestID": "req-v25",
        "sandbox_id": "sandbox-v25",
        "ret": { "ret_code": 0, "ret_msg": "ok" }
    }))
}

async fn spawn_service(capture: Capture) -> SandboxService {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("listener should bind");
    let addr = listener.local_addr().expect("listener addr");
    let app = Router::new()
        .route("/cube/sandbox", post(create_handler))
        .with_state(capture);
    tokio::spawn(async move {
        axum::serve(listener, app).await.expect("server should run");
    });

    SandboxService::new(
        CubeMasterClient::new(format!("http://{addr}"), reqwest::Client::new()),
        "cubebox".to_string(),
        "cube.app".to_string(),
    )
}

fn probe_sandbox(metadata: Option<std::collections::HashMap<String, String>>) -> NewSandbox {
    NewSandbox {
        template_id: "tpl-v25".to_string(),
        timeout: Some(30),
        lifecycle: None,
        auto_pause: None,
        auto_resume: None,
        secure: None,
        allow_internet_access: None,
        network: None,
        metadata,
        distribution_scope: None,
        env_vars: None,
        mcp: None,
        volume_mounts: None,
        backend: None,
    }
}

#[tokio::test]
async fn v25_create_mints_canonical_provider_incarnation_annotation() {
    let capture = Capture::default();
    let service = spawn_service(capture.clone()).await;

    service
        .create_sandbox(probe_sandbox(None))
        .await
        .expect("sandbox create should succeed");

    let body = capture
        .create_body
        .lock()
        .await
        .clone()
        .expect("create body should be captured");
    let raw = body["annotations"][PROVIDER_INCARNATION_ANNOTATION]
        .as_str()
        .expect("provider incarnation annotation missing");
    let parsed = Uuid::parse_str(raw).expect("provider incarnation should be UUID text");
    assert_eq!(parsed.get_version_num(), 4);
    assert_eq!(parsed.hyphenated().to_string(), raw);
}

#[tokio::test]
async fn v25_client_metadata_cannot_nominate_provider_incarnation_authority() {
    let capture = Capture::default();
    let service = spawn_service(capture.clone()).await;
    let attacker_value = "550e8400-e29b-41d4-a716-446655440000";
    let metadata = std::collections::HashMap::from([(
        PROVIDER_INCARNATION_ANNOTATION.to_string(),
        attacker_value.to_string(),
    )]);

    service
        .create_sandbox(probe_sandbox(Some(metadata)))
        .await
        .expect("sandbox create should succeed");

    let body = capture
        .create_body
        .lock()
        .await
        .clone()
        .expect("create body should be captured");
    assert_eq!(
        body["labels"][PROVIDER_INCARNATION_ANNOTATION],
        attacker_value,
        "client metadata should remain ordinary labels"
    );
    let internal = body["annotations"][PROVIDER_INCARNATION_ANNOTATION]
        .as_str()
        .expect("trusted provider incarnation annotation missing");
    assert_ne!(internal, attacker_value, "client metadata nominated authority");
    let parsed = Uuid::parse_str(internal).expect("internal incarnation should be UUID text");
    assert_eq!(parsed.get_version_num(), 4);
    assert_eq!(parsed.hyphenated().to_string(), internal);
}
