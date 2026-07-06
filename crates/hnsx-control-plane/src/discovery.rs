//! Agent endpoint discovery service.
//!
//! Returns live instances for a domain, optionally filtered by tags and region.
//! Expired instances are pruned before each query.

use tonic::{Request, Response, Status};

use crate::{
    proto::{DiscoverRequest, InstanceList, discovery_server::Discovery},
    store::SqliteStore,
};

#[derive(Clone)]
pub struct DiscoveryService {
    store: SqliteStore,
    heartbeat_timeout_ms: i64,
}

impl DiscoveryService {
    pub fn new(store: SqliteStore) -> Self {
        Self {
            store,
            heartbeat_timeout_ms: 60_000,
        }
    }

    /// Set the heartbeat timeout used when pruning stale instances.
    #[must_use]
    pub fn with_heartbeat_timeout_ms(mut self, ms: i64) -> Self {
        self.heartbeat_timeout_ms = ms;
        self
    }
}

#[tonic::async_trait]
impl Discovery for DiscoveryService {
    async fn discover(
        &self,
        request: Request<DiscoverRequest>,
    ) -> Result<Response<InstanceList>, Status> {
        crate::timed_grpc_async!("discover", async {
            let req = request.into_inner();
            self.store
                .expire_instances(self.heartbeat_timeout_ms)
                .await
                .map_err(|e| Status::internal(format!("failed to expire instances: {e}")))?;
            let instances = self
                .store
                .discover_instances(&req.domain_id, &req.tags, &req.region)
                .await
                .map_err(|e| Status::internal(format!("discovery failed: {e}")))?;
            crate::metrics::set_instance_count(&req.domain_id, instances.len());
            Ok(Response::new(InstanceList { instances }))
        })
    }
}

#[cfg(test)]
mod tests {
    use crate::proto::InstanceInfo;

    use super::*;

    #[tokio::test]
    async fn discover_filters_by_tags_and_region() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        store
            .register_instance(&InstanceInfo {
                instance_id: "i-1".into(),
                domain_id: "foo".into(),
                tags: vec!["blue".into()],
                region: "us-east".into(),
                capabilities: vec![],
            },
            )
            .await
            .unwrap();
        store
            .register_instance(&InstanceInfo {
                instance_id: "i-2".into(),
                domain_id: "foo".into(),
                tags: vec!["red".into()],
                region: "us-west".into(),
                capabilities: vec![],
            },
            )
            .await
            .unwrap();

        let svc = DiscoveryService::new(store);
        let resp = svc
            .discover(Request::new(DiscoverRequest {
                domain_id: "foo".into(),
                tags: vec!["blue".into()],
                region: "us-east".into(),
            }))
            .await
            .unwrap();
        let ids: Vec<String> = resp
            .into_inner()
            .instances
            .into_iter()
            .map(|i| i.instance_id)
            .collect();
        assert_eq!(ids, vec!["i-1"]);
    }
}
