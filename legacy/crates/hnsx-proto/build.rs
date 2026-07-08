fn main() {
    tonic_build::configure()
        .type_attribute(".", "#[derive(serde::Serialize, serde::Deserialize)]")
        .compile_protos(
            &["../../proto/hnsx/v1/control_plane.proto"],
            &["../../proto"],
        )
        .expect("compile proto");
}
