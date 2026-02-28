// trading engine - technical indicators via grpc
// implements rsi, macd, bollinger bands, ema, volume analysis

mod indicators;
mod server;

use server::proto::technical_indicators_server::TechnicalIndicatorsServer;
use tonic::transport::Server;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let addr = "0.0.0.0:50051".parse()?;
    let service = server::IndicatorService;

    println!("trading-engine v0.1.0");
    println!("grpc server listening on {}", addr);

    Server::builder()
        .add_service(TechnicalIndicatorsServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
