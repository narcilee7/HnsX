//! Domain packaging: pack a domain directory into a portable tarball.
//!
//! The package format is a gzip-compressed tar archive with a small JSON
//! manifest at the root (`hnsx-package.json`). It is intentionally simple and
//! not OCI-image-layout, so we can iterate before committing to a registry
//! protocol.

use std::fs::File;
use std::io::{BufReader, Read, Write};
use std::path::{Path, PathBuf};

use flate2::Compression;
use flate2::read::GzDecoder;
use flate2::write::GzEncoder;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tar::Archive;
use walkdir::WalkDir;

use crate::domain::DomainSpec;
use crate::error::{Error, Result};

/// Manifest stored at the root of every `.hnsx.tar` package.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PackageManifest {
    pub format: String,
    pub id: String,
    pub version: String,
    pub description: String,
    pub created_at: String,
    pub files: Vec<PackageFile>,
}

impl Default for PackageManifest {
    fn default() -> Self {
        Self {
            format: "hnsx-v1".into(),
            id: String::new(),
            version: String::new(),
            description: String::new(),
            created_at: String::new(),
            files: Vec::new(),
        }
    }
}

/// A single file entry inside the package.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PackageFile {
    pub path: String,
    pub size: u64,
    pub sha256: String,
}

const MANIFEST_NAME: &str = "hnsx-package.json";
const DOMAIN_YAML: &str = "domain.yaml";

/// Pack the domain at `domain_path` into a gzip-compressed tarball at `output`.
///
/// The domain file's parent directory is walked recursively. The resulting tar
/// has `hnsx-package.json` plus all files from that directory, preserving
/// relative paths.
///
/// # Errors
///
/// Returns an error if the domain file cannot be read, the directory cannot be
/// walked, or the output cannot be written.
pub fn pack_domain(domain_path: impl AsRef<Path>, output: impl AsRef<Path>) -> Result<()> {
    let domain_path = domain_path.as_ref();
    let output = output.as_ref();
    let base_dir = domain_path
        .parent()
        .ok_or_else(|| Error::InvalidSpec("domain path has no parent directory".into()))?;

    let spec_yaml = std::fs::read_to_string(domain_path)
        .map_err(Error::from)
        .map_err(|e| Error::InvalidSpec(format!("read domain yaml: {e}")))?;
    let spec: DomainSpec = serde_yaml::from_str(&spec_yaml)
        .map_err(|e| Error::InvalidSpec(format!("parse domain yaml: {e}")))?;

    // Collect all files first so we can compute hashes before writing.
    let mut collected: Vec<(PackageFile, Vec<u8>)> = Vec::new();
    for entry in WalkDir::new(base_dir).into_iter().filter_map(|e| e.ok()) {
        let entry_path = entry.path();
        if !entry.file_type().is_file() {
            continue;
        }
        let rel = entry_path
            .strip_prefix(base_dir)
            .map_err(|e| Error::InvalidSpec(format!("strip prefix: {e}")))?;
        let rel_str = rel.to_string_lossy().replace('\\', "/");
        if rel_str == MANIFEST_NAME {
            continue;
        }

        let mut buf = Vec::new();
        File::open(entry_path)
            .and_then(|mut f| f.read_to_end(&mut buf))
            .map_err(|e| Error::InvalidSpec(format!("read {}: {e}", entry_path.display())))?;

        let hash = format!("{:x}", Sha256::digest(&buf));
        collected.push((
            PackageFile {
                path: rel_str,
                size: buf.len() as u64,
                sha256: hash,
            },
            buf,
        ));
    }

    let manifest = PackageManifest {
        id: spec.id.clone(),
        version: spec.version.clone(),
        description: spec.description.clone(),
        created_at: chrono::Utc::now().to_rfc3339(),
        files: collected.iter().map(|(f, _)| f.clone()).collect(),
        ..Default::default()
    };

    let output_file = File::create(output).map_err(Error::from)?;
    let gz = GzEncoder::new(output_file, Compression::default());
    let mut tar = tar::Builder::new(gz);

    append_manifest(&mut tar, &manifest)?;

    for (pkg_file, data) in collected {
        let mut header = tar::Header::new_gnu();
        header
            .set_path(&pkg_file.path)
            .map_err(|e| Error::InvalidSpec(format!("tar path: {e}")))?;
        header.set_size(pkg_file.size);
        header.set_mode(0o644);
        header.set_cksum();
        tar.append(&header, data.as_slice())
            .map_err(|e| Error::InvalidSpec(format!("append {}: {e}", pkg_file.path)))?;
    }

    let gz = tar.into_inner().map_err(Error::from)?;
    gz.finish().map_err(Error::from)?;
    Ok(())
}

fn append_manifest(tar: &mut tar::Builder<impl Write>, manifest: &PackageManifest) -> Result<()> {
    let manifest_json = serde_json::to_string_pretty(manifest)
        .map_err(|e| Error::InvalidSpec(format!("serialize manifest: {e}")))?;
    let mut header = tar::Header::new_gnu();
    header
        .set_path(MANIFEST_NAME)
        .map_err(|e| Error::InvalidSpec(format!("tar path: {e}")))?;
    header.set_size(manifest_json.len() as u64);
    header.set_mode(0o644);
    header.set_cksum();
    tar.append(&header, manifest_json.as_bytes())
        .map_err(|e| Error::InvalidSpec(format!("append manifest: {e}")))?;
    Ok(())
}

/// Unpack a `.hnsx.tar` package into `dest_dir` and return the manifest plus
/// the path to the unpacked `domain.yaml`.
///
/// # Errors
///
/// Returns an error if the archive is malformed, the manifest is missing, or
/// the destination cannot be written.
pub fn unpack_domain(
    package: impl AsRef<Path>,
    dest_dir: impl AsRef<Path>,
) -> Result<(PackageManifest, PathBuf)> {
    let dest_dir = dest_dir.as_ref();
    std::fs::create_dir_all(dest_dir).map_err(Error::from)?;

    let file = File::open(package.as_ref()).map_err(Error::from)?;
    let gz = GzDecoder::new(BufReader::new(file));
    let mut archive = Archive::new(gz);
    archive.unpack(dest_dir).map_err(Error::from)?;

    let manifest_path = dest_dir.join(MANIFEST_NAME);
    let manifest_json = std::fs::read_to_string(&manifest_path)
        .map_err(|e| Error::InvalidSpec(format!("read manifest: {e}")))?;
    let manifest: PackageManifest = serde_json::from_str(&manifest_json)
        .map_err(|e| Error::InvalidSpec(format!("parse manifest: {e}")))?;

    let domain_yaml = dest_dir.join(DOMAIN_YAML);
    if !domain_yaml.exists() {
        return Err(Error::InvalidSpec(format!("package missing {DOMAIN_YAML}")));
    }
    Ok((manifest, domain_yaml))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn pack_and_unpack_round_trip() {
        let dir = tempdir().expect("tempdir");
        let domain_dir = dir.path().join("financial-analysis");
        std::fs::create_dir(&domain_dir).expect("create domain dir");
        std::fs::write(
            domain_dir.join("domain.yaml"),
            r#"
id: financial-analysis
version: 0.1.0
description: test
agents: []
workflow:
  entry: s1
  steps:
    - id: s1
      agent: a
"#,
        )
        .expect("write domain yaml");
        std::fs::write(domain_dir.join("prompt.txt"), "hello").expect("write resource");

        let out = dir.path().join("fa.hnsx.tar");
        pack_domain(domain_dir.join("domain.yaml"), &out).expect("pack");
        assert!(out.exists());

        let unpack_dir = dir.path().join("unpacked");
        let (manifest, domain_yaml) = unpack_domain(&out, &unpack_dir).expect("unpack");
        assert_eq!(manifest.id, "financial-analysis");
        assert_eq!(manifest.version, "0.1.0");
        assert!(domain_yaml.exists());
        assert!(unpack_dir.join("prompt.txt").exists());

        // Verify file list contains both files.
        let paths: Vec<&str> = manifest.files.iter().map(|f| f.path.as_str()).collect();
        assert!(paths.contains(&"domain.yaml"));
        assert!(paths.contains(&"prompt.txt"));
    }
}
