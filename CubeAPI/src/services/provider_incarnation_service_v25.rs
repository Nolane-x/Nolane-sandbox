// Copyright (c) 2024 Tencent Inc.
// SPDX-License-Identifier: Apache-2.0

use std::sync::Arc;

use axum::{extract::State, routing::{get, post}, Json, Router};
use serde_json::{Map, Value};
use tokio::sync::Mutex;
use uuid::Uuid;

use super::{
    provider_incarnation::PROVIDER_INCARNATION_ANNOTATION, sandboxes::SandboxService,
};
use crate::{cubemaster::CubeMasterClient, models::NewSandbox};

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

async fn spawn_router(app: Router) -> SandboxService {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("listener should bind");
    let addr = listener.local_addr().expect("listener addr");
    tokio::spawn(async move {
        axum::serve(listener, app).await.expect("server should run");
    });

    SandboxService::new(
        CubeMasterClient::new(format!("http://{addr}"), reqwest::Client::new()),
        "cubebox".to_string(),
        "cube.app".to_string(),
    )
}

async fn spawn_service(capture: Capture) -> SandboxService {
    spawn_router(
        Router::new()
            .route("/cube/sandbox", post(create_handler))
            .with_state(capture),
    )
    .await
}

async fn spawn_persisted_service(
    incarnation: Option<&str>,
    status: i32,
    with_update: bool,
) -> SandboxService {
    let incarnation = incarnation.map(str::to_owned);
    let info = get(move || {
        let incarnation = incarnation.clone();
        async move {
            let mut annotations = Map::new();
            if let Some(value) = incarnation {
                annotations.insert(
                    PROVIDER_INCARNATION_ANNOTATION.to_string(),
                    Value::String(value),
                );
            }
            Json(serde_json::json!({
                "requestID": "req-v25-info",
                "ret": { "ret_code": 0, "ret_msg": "ok" },
                "data": [{
                    "sandbox_id": "sandbox-v25",
                    "status": status,
                    "host_id": "",
                    "template_id": "tpl-v25",
                    "annotations": annotations,
                    "labels": {},
                    "containers": []
                }]
            }))
        }
    });

    let mut app = Router::new().route("/cube/sandbox/info", info);
    if with_update {
        app = app.route(
            "/cube/sandbox/update",
            post(|| async {
                Json(serde_json::json!({
                    "ret": { "ret_code": 0, "ret_msg": "ok" }
                }))
            }),
        );
    }
    spawn_router(app).await
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

    let sandbox = service
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

    let response = serde_json::to_value(sandbox).expect("sandbox response should serialize");
    assert_eq!(
        response.get("incarnationID").and_then(Value::as_str),
        Some(raw),
        "create response did not reuse the exact provider incarnation sent to CubeMaster"
    );
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

#[tokio::test]
async fn v25_get_projects_exact_persisted_provider_incarnation() {
    let persisted = "550e8400-e29b-41d4-a716-446655440000";
    let service = spawn_persisted_service(Some(persisted), 1, false).await;

    let detail = service
        .get_sandbox("sandbox-v25")
        .await
        .expect("persisted sandbox should load");
    let response = serde_json::to_value(detail).expect("sandbox detail should serialize");
    assert_eq!(
        response.get("incarnationID").and_then(Value::as_str),
        Some(persisted),
        "GET did not project the exact persisted provider incarnation"
    );
}

#[tokio::test]
async fn v25_get_omits_missing_or_malformed_provider_incarnation() {
    for persisted in [
        None,
        Some("not-a-uuid"),
        Some("550E8400-E29B-41D4-A716-446655440000"),
        Some("550e8400-e29b-11d4-a716-446655440000"),
    ] {
        let service = spawn_persisted_service(persisted, 1, false).await;
        let detail = service
            .get_sandbox("sandbox-v25")
            .await
            .expect("legacy/malformed sandbox should still be readable");
        let response = serde_json::to_value(detail).expect("sandbox detail should serialize");
        assert!(
            response.get("incarnationID").is_none(),
            "missing/malformed provider incarnation was upgraded into authority: {response}"
        );
    }
}

#[tokio::test]
async fn v25_connect_projects_exact_persisted_provider_incarnation_without_reminting() {
    let persisted = "550e8400-e29b-41d4-a716-446655440000";
    let service = spawn_persisted_service(Some(persisted), 1, false).await;

    let sandbox = service
        .connect_sandbox("sandbox-v25", None)
        .await
        .expect("running sandbox should connect");
    let response = serde_json::to_value(sandbox).expect("sandbox response should serialize");
    assert_eq!(
        response.get("incarnationID").and_then(Value::as_str),
        Some(persisted),
        "connect reminted or dropped the persisted provider incarnation"
    );
}

#[tokio::test]
async fn v25_resume_projects_exact_persisted_provider_incarnation_without_reminting() {
    let persisted = "550e8400-e29b-41d4-a716-446655440000";
    let service = spawn_persisted_service(Some(persisted), 1, true).await;

    let sandbox = service
        .resume_sandbox("sandbox-v25", None)
        .await
        .expect("sandbox should resume");
    let response = serde_json::to_value(sandbox).expect("sandbox response should serialize");
    assert_eq!(
        response.get("incarnationID").and_then(Value::as_str),
        Some(persisted),
        "resume reminted or dropped the persisted provider incarnation"
    );
}
