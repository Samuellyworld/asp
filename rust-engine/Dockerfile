# rust engine dockerfile - technical indicators via grpc

FROM rust:1.75-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache musl-dev protobuf-dev

# Copy cargo files first for better caching
COPY Cargo.toml Cargo.lock* ./

# Create dummy main.rs for dependency caching
RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo build --release || true
RUN rm -rf src

# Copy actual source code
COPY . .

# Build the binary
RUN cargo build --release

# final minimal image
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /app/target/release/trading-engine /app/trading-engine

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 50051

ENTRYPOINT ["/app/trading-engine"]
