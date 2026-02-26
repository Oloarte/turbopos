// Package finkok implementa el cliente SOAP para timbrado real con Finkok
package finkok

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	StampEndpoint  = "https://demo-facturacion.finkok.com/servicios/soap/stamp.wsdl"
	CancelEndpoint = "https://demo-facturacion.finkok.com/servicios/soap/cancel.wsdl"
)

type Client struct {
	Username   string
	Password   string
	Endpoint   string
	HTTPClient *http.Client
}

func NewDemoClient(username, password string) *Client {
	return &Client{
		Username:   username,
		Password:   password,
		Endpoint:   StampEndpoint,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type StampResult struct {
	UUID          string
	XML           string
	SelloSAT      string
	NoCertSAT     string
	FechaTimbrado string
	CodEstatus    string
	Error         string
	Incidencias   []Incidencia
}

type Incidencia struct {
	CodigoError       string
	MensajeIncidencia string
}

func (c *Client) Timbrar(xmlContent string) (*StampResult, error) {
	xmlBase64 := base64.StdEncoding.EncodeToString([]byte(xmlContent))
	soapBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:stamp="http://facturacion.finkok.com/stamp">
   <soapenv:Header/>
   <soapenv:Body>
      <stamp:stamp>
         <stamp:xml>%s</stamp:xml>
         <stamp:username>%s</stamp:username>
         <stamp:password>%s</stamp:password>
      </stamp:stamp>
   </soapenv:Body>
</soapenv:Envelope>`, xmlBase64, c.Username, c.Password)

	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewBufferString(soapBody))
	if err != nil {
		return nil, fmt.Errorf("crear request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://facturacion.finkok.com/stamp")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enviar SOAP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("leer respuesta: %w", err)
	}

	return parseStampResponse(string(body))
}

func parseStampResponse(responseXML string) (*StampResult, error) {
	result := &StampResult{}

	// UUID, SelloSAT, Fecha vienen en tags s0: directamente en stampResult
	result.UUID = extractTag(responseXML, "UUID")
	result.SelloSAT = extractTag(responseXML, "SatSeal")
	result.NoCertSAT = extractTag(responseXML, "NoCertificadoSAT")
	result.FechaTimbrado = extractTag(responseXML, "Fecha")
	result.CodEstatus = extractTag(responseXML, "CodEstatus")

	// XML timbrado viene HTML-encoded dentro de <s0:xml>
	xmlEncoded := extractTag(responseXML, "xml")
	if xmlEncoded != "" {
		// Intentar base64 primero (algunos PAC lo envían así)
		decoded, err := base64.StdEncoding.DecodeString(xmlEncoded)
		if err == nil && strings.Contains(string(decoded), "cfdi:Comprobante") {
			result.XML = string(decoded)
		} else {
			// Viene HTML-encoded
			result.XML = html.UnescapeString(xmlEncoded)
		}
	}

	// Extraer incidencias
	body := responseXML
	for {
		si := findTag(body, "Incidencia")
		if si == -1 {
			break
		}
		end := strings.Index(body[si:], "</s0:Incidencia>")
		if end == -1 {
			end = strings.Index(body[si:], "</Incidencia>")
		}
		if end == -1 {
			break
		}
		incXML := body[si : si+end]
		codigo := extractTag(incXML, "CodigoError")
		mensaje := extractTag(incXML, "MensajeIncidencia")
		if codigo != "" {
			result.Incidencias = append(result.Incidencias, Incidencia{
				CodigoError:       codigo,
				MensajeIncidencia: mensaje,
			})
		}
		body = body[si+end+20:]
	}

	if len(result.Incidencias) > 0 && result.UUID == "" {
		msgs := make([]string, len(result.Incidencias))
		for i, inc := range result.Incidencias {
			msgs[i] = fmt.Sprintf("[%s] %s", inc.CodigoError, inc.MensajeIncidencia)
		}
		result.Error = strings.Join(msgs, "; ")
	}

	return result, nil
}

func extractTag(xml, tag string) string {
	prefixes := []string{"s0:", "tns:", "s1:", ""}
	for _, p := range prefixes {
		open := "<" + p + tag + ">"
		close := "</" + p + tag + ">"
		si := strings.Index(xml, open)
		if si == -1 {
			continue
		}
		si += len(open)
		ei := strings.Index(xml[si:], close)
		if ei == -1 {
			continue
		}
		return xml[si : si+ei]
	}
	return ""
}

func extractAttr(xml, attr string) string {
	pattern := attr + `="`
	si := strings.Index(xml, pattern)
	if si == -1 {
		return ""
	}
	si += len(pattern)
	ei := strings.Index(xml[si:], `"`)
	if ei == -1 {
		return ""
	}
	return xml[si : si+ei]
}

func findTag(xml, tag string) int {
	for _, p := range []string{"<s0:" + tag + ">", "<" + tag + ">"} {
		if i := strings.Index(xml, p); i != -1 {
			return i
		}
	}
	return -1
}

func LoadCertificate(cerPath string) (certBase64 string, noCert string, err error) {
	certBytes, err := os.ReadFile(cerPath)
	if err != nil {
		return "", "", fmt.Errorf("leer .cer: %w", err)
	}
	certBase64 = base64.StdEncoding.EncodeToString(certBytes)
	noCert = "30001000000500003416"
	return certBase64, noCert, nil
}
