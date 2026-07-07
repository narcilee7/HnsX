use std::fs;
use std::path::Path;
use std::process::Command;

use anyhow::{Context, Result, bail};
use uuid::Uuid;

/// Deploy a domain artifact as a local Docker container.
///
/// Builds a minimal image containing the `hnsx` binary and the artifact, then
/// runs it in detached mode. The container runs `hnsx dev --artifact ...` and
/// registers with the control plane.
///
/// # Errors
///
/// Returns an error if Docker is not available, the artifact cannot be read,
/// or the build/run commands fail.
pub fn deploy(
    artifact: &Path,
    control_plane: &str,
    name: Option<&str>,
    port: Option<u16>,
) -> Result<()> {
    verify_docker()?;

    let artifact = artifact
        .canonicalize()
        .with_context(|| format!("canonicalize artifact {}", artifact.display()))?;
    let artifact_name = artifact
        .file_name()
        .and_then(|s| s.to_str())
        .context("artifact path must have a file name")?;

    // Resolve the hnsx binary that is running this command.
    let hnsx_bin = std::env::current_exe()
        .context("resolve current hnsx binary path")?;
    let hnsx_bin = hnsx_bin
        .canonicalize()
        .context("canonicalize hnsx binary path")?;

    let image_tag = format!("hnsx/runtime:{}", Uuid::new_v4());
    let container_name = name
        .map(|s| s.to_string())
        .unwrap_or_else(|| format!("hnsx-{}", Uuid::new_v4()));

    // Create a build context with the artifact and binary.
    let build_dir = tempfile::tempdir().context("create docker build context")?;
    let context_artifact = build_dir.path().join(artifact_name);
    fs::copy(&artifact, &context_artifact)
        .with_context(|| "copy artifact to build context".to_string())?;
    let context_bin = build_dir.path().join("hnsx");
    fs::copy(&hnsx_bin, &context_bin)
        .with_context(|| "copy hnsx binary to build context".to_string())?;

    let dockerfile = format!(
        r#"FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY hnsx /usr/local/bin/hnsx
RUN chmod +x /usr/local/bin/hnsx
COPY {artifact_name} /app/domain.hnsx.tar
ENV HNSX_CONTROL_PLANE={control_plane}
EXPOSE 8080
ENTRYPOINT ["hnsx", "dev", "--artifact", "/app/domain.hnsx.tar", "--bind", "0.0.0.0:8080", "--control-plane", "{control_plane}"]
"#,
        artifact_name = artifact_name,
        control_plane = control_plane
    );
    fs::write(build_dir.path().join("Dockerfile"), dockerfile)
        .context("write Dockerfile")?;

    // docker build.
    let mut build = Command::new("docker");
    build
        .arg("build")
        .arg("-t")
        .arg(&image_tag)
        .arg(build_dir.path());
    let status = build.status().context("run docker build")?;
    if !status.success() {
        bail!("docker build failed");
    }

    // docker run.
    let mut run = Command::new("docker");
    run.arg("run").arg("-d").arg("--name").arg(&container_name);
    if let Some(p) = port {
        run.arg("-p").arg(format!("{}:8080", p));
    }
    run.arg(&image_tag);

    let output = run
        .output()
        .context("run docker run")?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        bail!("docker run failed: {stderr}");
    }
    let container_id = String::from_utf8_lossy(&output.stdout).trim().to_string();

    println!("Deployed container {container_name} ({container_id})");
    println!("Image: {image_tag}");
    if let Some(p) = port {
        println!("Port mapping: {p} -> 8080");
    }
    Ok(())
}

fn verify_docker() -> Result<()> {
    let status = Command::new("docker")
        .arg("version")
        .status()
        .context("docker is not installed or not running")?;
    if !status.success() {
        bail!("docker daemon is not running");
    }
    Ok(())
}

/// Clean up a deployed container by name or id.
pub fn remove_container(name_or_id: &str) -> Result<()> {
    let status = Command::new("docker")
        .args(["rm", "-f", name_or_id,
        ])
        .status()
        .context("run docker rm")?;
    if !status.success() {
        bail!("docker rm failed for {name_or_id}");
    }
    Ok(())
}
