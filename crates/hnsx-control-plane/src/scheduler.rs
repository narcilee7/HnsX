//! Agent instance scheduler service.
//!
//! Tracks running agent instances via heartbeats and prunes instances that have
//! not reported within a configurable timeout.

use tonic::{Request, Response, Status};

use crate::{
    proto::{DomainRef, Empty, InstanceInfo, InstanceList, InstanceRef, scheduler_server::Scheduler},
    store::SqliteStore,
};

#[derive(Clone)]
pub struct SchedulerService {
    store: SqliteStore,
    heartbeat_timeout_ms: i64,
}

impl SchedulerService {
    pub fn new(store: SqliteStore, heartbeat_timeout_ms: i64) -> Self {
        Self {
            store,
            heartbeat_timeout_ms,
        }
    }
}

#[tonic::async_trait]
impl Scheduler for SchedulerService {
    async fn register_instance(
        &self,
        request: Request<InstanceInfo>,
    ) -> Result<Response<InstanceRef>, Status> {
        let info = request.into_inner();
        self.store
            .register_instance(&info)
            .await
            .map_err(|e| Status::internal(format!("failed to register instance: {e}")))?;
        Ok(Response::new(InstanceRef {
            instance_id: info.instance_id,
        }))
    }

    async fn heartbeat(
        &self,
        request: Request<InstanceRef>,
    ) -> Result<Response<Empty>, Status> {
        let req = request.into_inner();
        let ok = self
            .store
            .heartbeat(&req.instance_id)
            .await
            .map_err(|e| Status::internal(format!("heartbeat failed: {e}")))?;
        if !ok {
            return Err(Status::not_found(format!(
                "instance {} not registered",
                req.instance_id
            )));
        }
        Ok(Response::new(Empty {}))
    }

    async fn unregister_instance(
        &self,
        request: Request<InstanceRef>,
    ) -> Result<Response<Empty>, Status> {
        let req = request.into_inner();
        self.store
            .unregister_instance(&req.instance_id)
            .await
            .map_err(|e| Status::internal(format!("failed to unregister instance: {e}")))?;
        Ok(Response::new(Empty {}))
    }

    async fn list_instances(
        &self,
        request: Request<DomainRef>,
    ) -> Result<Response<InstanceList>, Status> {
        let req = request.into_inner();
        self.store
            .expire_instances(self.heartbeat_timeout_ms)
            .await
            .map_err(|e| Status::internal(format!("failed to expire instances: {e}")))?;
        let instances = self
            .store
            .list_instances(&req.id)
            .await
            .map_err(|e| Status::internal(format!("failed to list instances: {e}")))?;
        Ok(Response::new(InstanceList { instances }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn register_then_list() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let svc = SchedulerService::new(store, 60_000);
        svc.register_instance(Request::new(InstanceInfo {
            instance_id: "i-1".into(),
            domain_id: "foo".into(),
            tags: vec![],
            region: "".into(),
            capabilities: vec![],
        }))
        .await
        .unwrap();

        let resp = svc
            .list_instances(Request::new(DomainRef {
                id: "foo".into(),
                version: "1".into(),
            }))
            .await
            .unwrap();
        assert_eq!(resp.into_inner().instances.len(), 1);
    }
}
