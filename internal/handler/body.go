package handler

import (
	"encoding/base64"
	"errors"

	"github.com/aws/aws-lambda-go/events"
)

// rawBody devolve o corpo da requisicao ja decodificado. O API Gateway entrega
// o payload em base64 quando o classifica como binario, o que acontece por
// content-type -- entao a decodificacao nao pode ser assumida em nenhum dos
// dois sentidos.
func rawBody(req events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if !req.IsBase64Encoded {
		return []byte(req.Body), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		return nil, errors.New("invalid base64 body")
	}

	return decoded, nil
}
