# Imagem SO PARA DESENVOLVIMENTO LOCAL, usando o Lambda Runtime Interface
# Emulator. O artefato de producao e um zip com o binario `bootstrap`, montado
# pelo CI e publicado com `aws lambda update-function-code` -- ver
# .github/workflows/ci.yml.
FROM golang:1.26.1-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bootstrap ./cmd/lambda

FROM public.ecr.aws/lambda/provided:al2023
COPY --from=build /bootstrap /var/runtime/bootstrap
CMD ["bootstrap"]
