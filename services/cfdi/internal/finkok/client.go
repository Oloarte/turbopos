// Package finkok implementa el cliente SOAP para timbrado real con Finkok
package finkok

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/youmark/pkcs8"
)

const (
	StampEndpoint  = "https://demo-facturacion.finkok.com/servicios/soap/stamp.wsdl"
	CancelEndpoint = "https://demo-facturacion.finkok.com/servicios/soap/cancel"
)

type Client struct {
	Username   string
	Password   string
	Endpoint   string
	HTTPClient *http.Client
}

func NewDemoClient(user, pass string) *Client {
	return &Client{
		Username:   user,
		Password:   pass,
		Endpoint:   StampEndpoint,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type StampResult struct {
	Error string
	UUID          string
	SelloSAT      string
	NoCertSAT     string
	FechaTimbrado string
	CodEstatus    string
	XML           string
	Incidencias   []Incidencia
}

type Incidencia struct {
	CodigoError      string
	MensajeIncidencia string
}

type CancelResult struct {
	Error string
	Status  string
	Mensaje string
	Acuse   string
	Folios  []FolioResult
}

type FolioResult struct {
	UUID               string
	EstatusUUID        string
	EstatusCancelacion string
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
	// Finkok requiere cert=PEM en base64, key=RSA PRIVATE KEY PEM sin encriptar en base64
	certPEMB64, keyPEMB64, err := prepararCertKey([]byte(certB64), keyBytes, keyPassword)
	if err != nil {
		return nil, fmt.Errorf("preparar cert/key: %w", err)
	}

	folioSus := ""
	if motivo == "01" && uuidReemplazo != "" {
		folioSus = fmt.Sprintf(` FolioSustitucion="%s"`, uuidReemplazo)
	}
	uuidNode := fmt.Sprintf(`<s0:UUID UUID="%s" Motivo="%s"%s/>`, uuid, motivo, folioSus)

	soapBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:canc="http://facturacion.finkok.com/cancel" xmlns:s0="apps.services.soap.core.views">
   <soapenv:Header/>
   <soapenv:Body>
      <canc:cancel>
         <canc:UUIDS>%s</canc:UUIDS>
         <canc:username>%s</canc:username>
         <canc:password>%s</canc:password>
         <canc:taxpayer_id>%s</canc:taxpayer_id>
         <canc:cer>%s</canc:cer>
         <canc:key>%s</canc:key>
         <canc:store_pending>0</canc:store_pending>
      </canc:cancel>
   </soapenv:Body>
</soapenv:Envelope>`, uuidNode, c.Username, c.Password, rfc, certPEMB64, keyPEMB64)

	req, err := http.NewRequest("POST", CancelEndpoint, bytes.NewBufferString(soapBody))
	if err != nil { return nil, fmt.Errorf("crear request cancelar: %w", err) }
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "cancel")

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, fmt.Errorf("enviar SOAP cancelar: %w", err) }
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { return nil, fmt.Errorf("leer respuesta cancelar: %w", err) }

	return parseCancelResponse(string(body))
}

// prepararCertKey convierte cert DER y key PKCS8 encriptado al formato PEM en base64 que espera Finkok
func prepararCertKey(certDER []byte, keyBytes []byte, password string) (certB64, keyB64 string, err error) {
	// Si certDER ya viene como base64, decodificar
	decoded, e := base64.StdEncoding.DecodeString(string(certDER))
	if e == nil {
		certDER = decoded
	}

	// Cert: PEM en base64
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certB64 = base64.StdEncoding.EncodeToString(certPEM)

	// Key: desencriptar PKCS8 DER del SAT y serializar como PKCS1 PEM sin encriptar
	key, err := pkcs8.ParsePKCS8PrivateKey(keyBytes, []byte(password))
	if err != nil {
		return "", "", fmt.Errorf("desencriptar key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", "", fmt.Errorf("key no es RSA")
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})
	keyB64 = base64.StdEncoding.EncodeToString(keyPEM)
	return
}

func parseCancelResponse(responseXML string) (*CancelResult, error) {
	result := &CancelResult{}
	result.Status = extractTag(responseXML, "CodEstatus")
	result.Acuse  = extractTag(responseXML, "Acuse")

	// Parsear folios
	body := responseXML
	for {
		si := strings.Index(body, "<s0:Folio>")
		if si == -1 { break }
		end := strings.Index(body[si:], "</s0:Folio>")
		if end == -1 { break }
		folioXML := body[si : si+end]
		result.Folios = append(result.Folios, FolioResult{
			UUID:               extractTag(folioXML, "UUID"),
			EstatusUUID:        extractTag(folioXML, "EstatusUUID"),
			EstatusCancelacion: extractTag(folioXML, "EstatusCancelacion"),
		})
		body = body[si+end+11:]
	}

	if len(result.Folios) > 0 {
		result.Status  = result.Folios[0].EstatusCancelacion
		result.Mensaje = result.Folios[0].EstatusCancelacion
	} else if result.Status == "" {
		fault := extractTag(responseXML, "faultstring")
		if fault != "" {
			return nil, fmt.Errorf("SOAP fault: %s", fault)
		}
	}
	return result, nil
}

// LoadCertificate carga y decodifica un certificado .cer del SAT
func LoadCertificate(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil { return "", "", fmt.Errorf("leer cert: %w", err) }
	cert, err := x509.ParseCertificate(data)
	if err != nil { return "", "", fmt.Errorf("parsear cert: %w", err) }
	noCert := cert.SerialNumber.String()
	// handled above
	return base64.StdEncoding.EncodeToString(data), noCert, nil
}

func parseStampResponse(responseXML string) (*StampResult, error) {
	result := &StampResult{}

	result.UUID          = extractTag(responseXML, "UUID")
	result.SelloSAT      = extractTag(responseXML, "SatSeal")
	result.NoCertSAT     = extractTag(responseXML, "NoCertificadoSAT")
	result.FechaTimbrado = extractTag(responseXML, "Fecha")
	result.CodEstatus    = extractTag(responseXML, "CodEstatus")

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
		codigo  := extractTag(incXML, "CodigoError")
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
		return result, fmt.Errorf("incidencias: %s", strings.Join(msgs, "; "))
	}
	if result.UUID == "" && result.CodEstatus != "" && result.CodEstatus != "S - Comprobante timbrado satisfactoriamente" {
		return result, fmt.Errorf("timbrado fallido: %s", result.CodEstatus)
	}
	return result, nil
}

func extractTag(xml, tag string) string {
	open  := "<" + tag + ">"
	close := "</" + tag + ">"
	si := strings.Index(xml, open)
	if si == -1 {
		// buscar con namespace
		si2 := strings.Index(xml, ":"+tag+">")
		if si2 == -1 { return "" }
		// encontrar apertura del namespace
		start := strings.LastIndex(xml[:si2], "<")
		if start == -1 { return "" }
		si = start
		open = xml[start : si2+len(tag)+2]
		close = "</" + xml[si2-strings.LastIndex(xml[:si2], ":")-1:si2] + tag + ">"
		_ = close
		ei := strings.Index(xml[si2+len(tag)+1:], "<")
		if ei == -1 { return "" }
		return xml[si2+len(tag)+2 : si2+len(tag)+1+ei]
	}
	ei := strings.Index(xml[si+len(open):], close)
	if ei == -1 { return "" }
	return xml[si+len(open) : si+len(open)+ei]
}

func findTag(xml, tag string) int {
	patterns := []string{"<s0:" + tag, "<" + tag}
	for _, p := range patterns {
		if i := strings.Index(xml, p); i != -1 { return i }
	}
	return -1
}
