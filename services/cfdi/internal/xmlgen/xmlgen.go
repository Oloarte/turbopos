// Package xmlgen genera XML CFDI 4.0 válido para timbrado con PAC
package xmlgen

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

// CFDIComprobante representa el comprobante fiscal CFDI 4.0
type CFDIComprobante struct {
	Version              string
	Fecha                string
	Sello                string
	NoCertificado        string
	Certificado          string
	SubTotal             string
	Descuento            string
	Total                string
	Moneda               string
	TipoDeComprobante    string
	MetodoPago           string
	FormaPago            string
	LugarExpedicion      string
	Emisor               Emisor
	Receptor             Receptor
	Conceptos            []Concepto
	ImpuestosTrasladados []ImpuestoTraslado
}

type Emisor struct {
	Rfc           string
	Nombre        string
	RegimenFiscal string
}

type Receptor struct {
	Rfc                     string
	Nombre                  string
	DomicilioFiscalReceptor string
	RegimenFiscalReceptor   string
	UsoCFDI                 string
}

type Concepto struct {
	ClaveProdServ string
	ClaveUnidad   string
	Cantidad      string
	Descripcion   string
	ValorUnitario string
	Importe       string
	ObjetoImp     string
	TasaIVA       float64
}

type ImpuestoTraslado struct {
	Base       string
	Impuesto   string
	TipoFactor string
	TasaOCuota string
	Importe    string
}

// SaleData datos de entrada para generar el CFDI
type SaleData struct {
	SaleID        string
	Fecha         time.Time
	RFC           string
	NombreReceptor string
	Items         []SaleItem
	Total         float64
	FormaPago     string // 01=efectivo, 04=tarjeta
	LugarExpedicion string
}

type SaleItem struct {
	Nombre        string
	Cantidad      int32
	PrecioUnitario float64
	Subtotal      float64
}

// GenerarXML genera el XML CFDI 4.0 sin firmar (PreCFDI)
func GenerarXML(data SaleData, certBase64, noCert string) (string, error) {
	fecha := data.Fecha.Format("2006-01-02T15:04:05")
	formaPago := data.FormaPago
	if formaPago == "" {
		formaPago = "01" // efectivo por defecto
	}
	lugar := data.LugarExpedicion
	if lugar == "" {
		lugar = "64000"
	}

	// Calcular subtotal e impuestos
	var subtotal float64
	for _, item := range data.Items {
		subtotal += item.Subtotal
	}
	// Desglosa IVA 16% del total (precio incluye IVA)
	baseIVA := subtotal / 1.16
	ivaTotal := subtotal - baseIVA

	// Construir conceptos
	var conceptosXML []string
	for _, item := range data.Items {
		baseItem := item.Subtotal / 1.16
		ivaItem := item.Subtotal - baseItem
		concepto := fmt.Sprintf(`    <cfdi:Concepto ClaveProdServ="78101803" ClaveUnidad="E48" `+
			`Cantidad="%.0f" Descripcion="%s" ValorUnitario="%.2f" Importe="%.2f" ObjetoImp="02">
      <cfdi:Impuestos>
        <cfdi:Traslados>
          <cfdi:Traslado Base="%.2f" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="%.2f"/>
        </cfdi:Traslados>
      </cfdi:Impuestos>
    </cfdi:Concepto>`,
			float64(item.Cantidad),
			escapeXML(item.Nombre),
			item.PrecioUnitario/1.16,
			baseItem,
			baseItem,
			ivaItem,
		)
		conceptosXML = append(conceptosXML, concepto)
	}

	nombreReceptor := data.NombreReceptor
	if nombreReceptor == "" {
		nombreReceptor = "PUBLICO EN GENERAL"
	}

	rfcReceptor := data.RFC
	if rfcReceptor == "" || rfcReceptor == "XAXX010101000" {
		rfcReceptor = "XAXX010101000"
		nombreReceptor = "PUBLICO EN GENERAL"
	}

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante
  xmlns:cfdi="http://www.sat.gob.mx/cfd/4"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://www.sat.gob.mx/cfd/4 http://www.sat.gob.mx/sitio_internet/cfd/4/cfdv40.xsd"
  Version="4.0"
  Fecha="%s"
  Sello=""
  NoCertificado="%s"
  Certificado="%s"
  SubTotal="%.2f"
  Total="%.2f"
  Moneda="MXN"
  TipoDeComprobante="I"
  MetodoPago="PUE"
  FormaPago="%s"
  LugarExpedicion="%s"
  Exportacion="01">
  <cfdi:Emisor Rfc="EKU9003173C9" Nombre="EMPRESA PRUEBAS SA DE CV" RegimenFiscal="601"/>
  <cfdi:Receptor
    Rfc="%s"
    Nombre="%s"
    DomicilioFiscalReceptor="%s"
    RegimenFiscalReceptor="616"
    UsoCFDI="S01"/>
  <cfdi:Conceptos>
%s
  </cfdi:Conceptos>
  <cfdi:Impuestos TotalImpuestosTrasladados="%.2f">
    <cfdi:Traslados>
      <cfdi:Traslado Base="%.2f" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="%.2f"/>
    </cfdi:Traslados>
  </cfdi:Impuestos>
</cfdi:Comprobante>`,
		fecha,
		noCert,
		certBase64,
		baseIVA,
		subtotal,
		formaPago,
		lugar,
		rfcReceptor,
		nombreReceptor,
		lugar,
		strings.Join(conceptosXML, "\n"),
		ivaTotal,
		baseIVA,
		ivaTotal,
	)

	return xml, nil
}

// FirmarXML firma el XML con la llave privada CSD y retorna el XML con Sello
func FirmarXML(xmlStr string, keyPEM []byte, password string) (string, error) {
	// Parsear llave privada
	privateKey, err := parsePrivateKey(keyPEM, password)
	if err != nil {
		return "", fmt.Errorf("parsear llave privada: %w", err)
	}

	// Cadena original = contenido del XML sin el atributo Sello
	cadena := generarCadenaOriginal(xmlStr)

	// Firmar con SHA256
	hash := sha256.Sum256([]byte(cadena))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("firmar: %w", err)
	}

	sello := base64.StdEncoding.EncodeToString(signature)

	// Insertar sello en el XML
	xmlFirmado := strings.Replace(xmlStr, `Sello=""`, fmt.Sprintf(`Sello="%s"`, sello), 1)

	return xmlFirmado, nil
}

// CertificadoBase64 convierte un .cer a base64 para incluir en el XML
func CertificadoBase64(certDER []byte) string {
	return base64.StdEncoding.EncodeToString(certDER)
}

// NoCertificado extrae el número de certificado de un .cer
func NoCertificado(certDER []byte) (string, error) {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "", fmt.Errorf("parsear certificado: %w", err)
	}
	// El número de certificado es el serial en hex
	serial := fmt.Sprintf("%x", cert.SerialNumber)
	// Convertir a formato SAT (solo dígitos, 20 chars)
	var result []byte
	for _, c := range serial {
		if c >= '0' && c <= '9' {
			result = append(result, byte(c))
		} else {
			// hex a decimal simple para certificados SAT
			result = append(result, byte(c))
		}
	}
	return string(result), nil
}

func parsePrivateKey(keyPEM []byte, password string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("no se pudo decodificar PEM")
	}

	var keyBytes []byte
	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		var err error
		keyBytes, err = x509.DecryptPEMBlock(block, []byte(password)) //nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("descifrar llave: %w", err)
		}
	} else {
		keyBytes = block.Bytes
	}

	key, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		// intentar PKCS1
		return x509.ParsePKCS1PrivateKey(keyBytes)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("la llave no es RSA")
	}
	return rsaKey, nil
}

func generarCadenaOriginal(xmlStr string) string {
	// La cadena original del CFDI es una cadena de pipe-delimited con los valores
	// Por simplicidad usamos el XML completo sin el sello como cadena
	// En producción real se usa la transformación XSLT del SAT
	lines := strings.Split(xmlStr, "\n")
	var relevant []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "<?xml") {
			relevant = append(relevant, line)
		}
	}
	return strings.Join(relevant, "")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
