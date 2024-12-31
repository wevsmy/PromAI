FROM docker.io/library/golang:1.23.4-alpine3.20 AS builder

WORKDIR /build
COPY . .
RUN go env -w GO111MODULE=on &&  go mod download && go build && ls -la /build

FROM docker.io/alpine:3.21.0
# 添加标识信息
LABEL version="1.0" \
      description="Prometheus Automated Inspection" \
      maintainer="Kubehan"
WORKDIR /app
COPY --from=builder /build/PromAI /app/
COPY --from=builder /build/config /app/config/
COPY --from=builder /build/outputs /app/outputs/
COPY --from=builder /build/reports /app/reports/
COPY --from=builder /build/templates /app/templates/
EXPOSE 8091
# 运行应用程序
CMD ["./PromAI", "-port", "8091"]