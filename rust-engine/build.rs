// compiles the proto file for the grpc service
fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::compile_protos("../proto/indicators.proto")?;
    Ok(())
}
