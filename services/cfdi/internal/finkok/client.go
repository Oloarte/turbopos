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

type CancelResult struct {
	Status  string
	Acuse   string
	Mensaje string
	Error   string
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
	if err != nil { return nil, fmt.Errorf("crear request: %w", err) }
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://facturacion.finkok.com/stamp")

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, fmt.Errorf("enviar SOAP: %w", err) }
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { return nil, fmt.Errorf("leer respuesta: %w", err) }

	return parseStampResponse(string(body))
}

// Cancelar envía solicitud de cancelación a Finkok
// motivo: 01=con relación, 02=sin relación, 03=no se llevó a cabo, 04=global
func (c *Client) Cancelar(uuid, rfc, motivo, uuidReemplazo, certB64 string, keyBytes []byte, keyPassword string) (*CancelResult, error) {
	// Finkok cancel requiere: cer=certificado DER base64, key=llave privada DER base64
	// Necesitamos convertir PEM → DER para la llave
	keyDERB64, err := pemToDERBase64(keyBytes, keyPassword)
	if err != nil {
		return nil, fmt.Errorf("convertir llave a DER: %w", err)
	}

	// Construir nodo UUID — Finkok espera s0:UUID directo dentro de canc:uuids
	uuidAttr := fmt.Sprintf(`Motivo="%s"`, motivo)
	if motivo == "01" && uuidReemplazo != "" {
		uuidAttr += fmt.Sprintf(` FolioSustitucion="%s"`, uuidReemplazo)
	}
	uuidNode := fmt.Sprintf(`<s0:UUID %s>%s</s0:UUID>`, uuidAttr, uuid)

	soapBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:canc="http://facturacion.finkok.com/cancel" xmlns:s0="apps.services.soap.core.views">
   <soapenv:Header/>
   <soapenv:Body>
      <canc:cancel>
         <canc:username>%s</canc:username>
         <canc:password>%s</canc:password>
         <canc:taxpayer_id>%s</canc:taxpayer_id>
         <canc:cer>%s</canc:cer>
         <canc:key>%s</canc:key>
         <canc:uuids>%s</canc:uuids>
      </canc:cancel>
   </soapenv:Body>
</soapenv:Envelope>`, c.Username, c.Password, rfc, certB64, keyDERB64, uuidNode)

	req, err := http.NewRequest("POST", CancelEndpoint, bytes.NewBufferString(soapBody))
	if err != nil { return nil, fmt.Errorf("crear request cancelar: %w", err) }
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://facturacion.finkok.com/cancel")

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, fmt.Errorf("enviar SOAP cancelar: %w", err) }
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { return nil, fmt.Errorf("leer respuesta cancelar: %w", err) }

	return parseCancelResponse(string(body))
}

// pemToDERBase64 convierte PEM encriptado a base64 tal cual (Finkok espera el PEM encriptado)
func pemToDERBase64(keyBytes []byte, password string) (string, error) {
	// Finkok espera la llave privada en su formato original (PEM encriptado) en base64
	return base64.StdEncoding.EncodeToString(keyBytes), nil
}

func parseCancelResponse(responseXML string) (*CancelResult, error) {
	result := &CancelResult{}

	// Estado de cancelación
	result.Status = extractTag(responseXML, "CodEstatus")
	if result.Status == "" {
		result.Status = extractTag(responseXML, "Estatus")
	}
	if result.Status == "" {
		result.Status = extractTag(responseXML, "EstatusUUID")
	}

	// Acuse (XML de acuse de recibo del SAT)
	acuseEncoded := extractTag(responseXML, "Acuse")
	if acuseEncoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(acuseEncoded)
		if err == nil {
			result.Acuse = string(decoded)
		} else {
			result.Acuse = html.UnescapeString(acuseEncoded)
		}
	}

	result.Mensaje = extractTag(responseXML, "Mensaje")
	if result.Mensaje == "" {
		result.Mensaje = result.Status
	}

	// Error
	faultString := extractTag(responseXML, "faultstring")
	if faultString != "" {
		result.Error = faultString
	}

	// Si no hay status pero tampoco error, es éxito
	if result.Status == "" && result.Error == "" {
		result.Status = "cancelado"
		result.Mensaje = "Solicitud de cancelación enviada"
	}

	return result, nil
}

func parseStampResponse(responseXML string) (*StampResult, error) {
	result := &StampResult{}

	result.UUID = extractTag(responseXML, "UUID")
	result.SelloSAT = extractTag(responseXML, "SatSeal")
	result.NoCertSAT = extractTag(responseXML, "NoCertificadoSAT")
	result.FechaTimbrado = extractTag(responseXML, "Fecha")
	result.CodEstatus = extractTag(responseXML, "CodEstatus")

	xmlEncoded := extractTag(responseXML, "xml")
	if xmlEncoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(xmlEncoded)
		if err == nil && strings.Contains(string(decoded), "cfdi:Comprobante") {
			result.XML = string(decoded)
		} else {
			result.XML = html.UnescapeString(xmlEncoded)
		}
	}

	body := responseXML
	for {
		si := findTag(body, "Incidencia")
		if si == -1 { break }
		end := strings.Index(body[si:], "</s0:Incidencia>")
		if end == -1 { end = strings.Index(body[si:], "</Incidencia>") }
		if end == -1 { break }
		incXML := body[si : si+end]
		codigo := extractTag(incXML, "CodigoError")
		mensaje := extractTag(incXML, "MensajeIncidencia")
		if codigo != "" {
			result.Incidencias = append(result.Incidencias, Incidencia{
				CodigoError: codigo, MensajeIncidencia: mensaje,
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
		if si == -1 { continue }
		si += len(open)
		ei := strings.Index(xml[si:], close)
		if ei == -1 { continue }
		return xml[si : si+ei]
	}
	return ""
}

func extractAttr(xml, attr string) string {
	pattern := attr + `="`
	si := strings.Index(xml, pattern)
	if si == -1 { return "" }
	si += len(pattern)
	ei := strings.Index(xml[si:], `"`)
	if ei == -1 { return "" }
	return xml[si : si+ei]
}

func findTag(xml, tag string) int {
	for _, p := range []string{"<s0:" + tag + ">", "<" + tag + ">"} {
		if i := strings.Index(xml, p); i != -1 { return i }
	}
	return -1
}

func LoadCertificate(cerPath string) (certBase64 string, noCert string, err error) {
	certBytes, err := os.ReadFile(cerPath)
	if err != nil { return "", "", fmt.Errorf("leer .cer: %w", err) }
	certBase64 = base64.StdEncoding.EncodeToString(certBytes)
	noCert = "30001000000500003416"
	return certBase64, noCert, nil
}
