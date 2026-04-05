// compiles the proto file for the grpc service
fn main() -> Result<(), Box<dyn std::error::Error>> {
    // in Docker the proto is at /proto, locally at ../proto
    let proto_path = if std::path::Path::new("/proto/indicators.proto").exists() {
        "/proto/indicators.proto"
    } else {
        "../proto/indicators.proto"
    };
    tonic_build::compile_protos(proto_path)?;
    Ok(())
}
