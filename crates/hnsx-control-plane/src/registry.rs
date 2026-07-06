//! Domain registry service.
//!
//! Persists registered domains in the shared `SqliteStore` keyed by
//! `(id, version)`.

use tonic::{Request, Response, Status};

use crate::{
    proto::{DomainList, DomainRef, DomainSpec, Empty, registry_server::Registry},
    store::SqliteStore,
};

#[derive(Clone)]
pub struct RegistryService {
    store: SqliteStore,
}

impl RegistryService {
    pub fn new(store: SqliteStore) -> Self {
        Self { store }
    }
}

#[tonic::async_trait]
impl Registry for RegistryService {
    async fn register_domain(
        &self,
        request: Request<DomainSpec>,
    ) -> Result<Response<DomainRef>, Status> {
        let spec = request.into_inner();
        self.store
            .register_domain(&spec)
            .await
            .map_err(|e| Status::internal(format!("failed to register domain: {e}")))?;
        Ok(Response::new(DomainRef {
            id: spec.id,
            version: spec.version,
        }))
    }

    async fn unregister_domain(
        &self,
        request: Request<DomainRef>,
    ) -> Result<Response<Empty>, Status> {
        let req = request.into_inner();
        self.store
            .unregister_domain(&req.id, &req.version)
            .await
            .map_err(|e| Status::internal(format!("failed to unregister domain: {e}")))?;
        Ok(Response::new(Empty {}))
    }

    async fn list_domains(
        &self,
        _request: Request<Empty>,
    ) -> Result<Response<DomainList>, Status> {
        let domains = self
            .store
            .list_domains()
            .await
            .map_err(|e| Status::internal(format!("failed to list domains: {e}")))?;
        Ok(Response::new(DomainList { domains }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn register_then_list() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let svc = RegistryService::new(store);
        svc.register_domain(Request::new(DomainSpec {
            id: "foo".into(),
            version: "1".into(),
            yaml_body: "id: foo".into(),
        }))
        .await
        .unwrap();

        let resp = svc.list_domains(Request::new(Empty {})).await.unwrap();
        assert_eq!(resp.into_inner().domains.len(), 1);
    }

    #[tokio::test]
    async fn unregister_removes_domain() {
        let store = SqliteStore::open_in_memory().await.unwrap();
        let svc = RegistryService::new(store);
        svc.register_domain(Request::new(DomainSpec {
            id: "foo".into(),
            version: "1".into(),
            yaml_body: "id: foo".into(),
        }))
        .await
        .unwrap();

        svc.unregister_domain(Request::new(DomainRef {
            id: "foo".into(),
            version: "1".into(),
        }))
        .await
        .unwrap();

        let resp = svc.list_domains(Request::new(Empty {})).await.unwrap();
        assert!(resp.into_inner().domains.is_empty());
    }
}
