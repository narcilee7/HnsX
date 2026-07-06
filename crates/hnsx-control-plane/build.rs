fn main() {
    tonic_build::configure()
        .compile_protos(
            &["../../proto/hnsx/v1/control_plane.proto"],
            &["../../proto"],
        )
        .expect("compile proto");
}
