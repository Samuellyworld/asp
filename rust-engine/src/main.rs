// trading engine - technical indicators via grpc
// implements rsi, macd, bollinger bands, ema, volume analysis

mod indicators;
mod server;

use server::proto::technical_indicators_server::TechnicalIndicatorsServer;
use tonic::transport::Server;
use tracing_subscriber::{fmt, EnvFilter};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // structured JSON logging (set RUST_LOG=debug for verbose)
    fmt()
        .json()
        .with_env_filter(
            EnvFilter::from_default_env().add_directive("trading_engine=info".parse()?),
        )
        .init();

    let addr = "0.0.0.0:50051".parse()?;
    let service = server::IndicatorService;

    tracing::info!(version = "0.1.0", "trading-engine starting");
    tracing::info!(%addr, "grpc server listening");

    Server::builder()
        .add_service(TechnicalIndicatorsServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
