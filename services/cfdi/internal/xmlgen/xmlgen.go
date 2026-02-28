package xmlgen

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

type SaleData struct {
	SaleID          string
	Fecha           time.Time
	RFC             string
	NombreReceptor  string
	Items           []SaleItem
	Total           float64
	FormaPago       string
	LugarExpedicion string
	CodigoPostalReceptor string
}

type SaleItem struct {
	Nombre         string
	Cantidad       int32
	PrecioUnitario float64
	Subtotal       float64
}

// --- Structs para parsear CFDI 4.0 ---

type Comprobante struct {
	XMLName           xml.Name           `xml:"Comprobante"`
	Version           string             `xml:"Version,attr"`
	Fecha             string             `xml:"Fecha,attr"`
	FormaPago         string             `xml:"FormaPago,attr"`
	NoCertificado     string             `xml:"NoCertificado,attr"`
	Certificado       string             `xml:"Certificado,attr"`
	Sello             string             `xml:"Sello,attr"`
	SubTotal          string             `xml:"SubTotal,attr"`
	Descuento         string             `xml:"Descuento,attr"`
	Moneda            string             `xml:"Moneda,attr"`
	TipoCambio        string             `xml:"TipoCambio,attr"`
	Total             string             `xml:"Total,attr"`
	TipoDeComprobante string             `xml:"TipoDeComprobante,attr"`
	Exportacion       string             `xml:"Exportacion,attr"`
	MetodoPago        string             `xml:"MetodoPago,attr"`
	LugarExpedicion   string             `xml:"LugarExpedicion,attr"`
	Confirmacion      string             `xml:"Confirmacion,attr"`
	InformacionGlobal *InformacionGlobal `xml:"InformacionGlobal"`
	Emisor            Emisor             `xml:"Emisor"`
	Receptor          Receptor           `xml:"Receptor"`
	Conceptos         Conceptos          `xml:"Conceptos"`
	Impuestos         *ImpuestosGlobal   `xml:"Impuestos"`
}

type InformacionGlobal struct {
	Periodicidad string `xml:"Periodicidad,attr"`
	Meses        string `xml:"Meses,attr"`
	Ano          string `xml:"Ano,attr"`
}

type Emisor struct {
	Rfc              string `xml:"Rfc,attr"`
	Nombre           string `xml:"Nombre,attr"`
	RegimenFiscal    string `xml:"RegimenFiscal,attr"`
	FacAtrAdquirente string `xml:"FacAtrAdquirente,attr"`
}

type Receptor struct {
	Rfc                     string `xml:"Rfc,attr"`
	Nombre                  string `xml:"Nombre,attr"`
	DomicilioFiscalReceptor string `xml:"DomicilioFiscalReceptor,attr"`
	ResidenciaFiscal        string `xml:"ResidenciaFiscal,attr"`
	NumRegIdTrib            string `xml:"NumRegIdTrib,attr"`
	RegimenFiscalReceptor   string `xml:"RegimenFiscalReceptor,attr"`
	UsoCFDI                 string `xml:"UsoCFDI,attr"`
}

type Conceptos struct {
	Conceptos []Concepto `xml:"Concepto"`
}

type Concepto struct {
	ClaveProdServ    string             `xml:"ClaveProdServ,attr"`
	NoIdentificacion string             `xml:"NoIdentificacion,attr"`
	Cantidad         string             `xml:"Cantidad,attr"`
	ClaveUnidad      string             `xml:"ClaveUnidad,attr"`
	Unidad           string             `xml:"Unidad,attr"`
	Descripcion      string             `xml:"Descripcion,attr"`
	ValorUnitario    string             `xml:"ValorUnitario,attr"`
	Importe          string             `xml:"Importe,attr"`
	Descuento        string             `xml:"Descuento,attr"`
	ObjetoImp        string             `xml:"ObjetoImp,attr"`
	Impuestos        *ImpuestosConcepto `xml:"Impuestos"`
}

type ImpuestosConcepto struct {
	Traslados   []TrasladoConcepto  `xml:"Traslados>Traslado"`
	Retenciones []RetencionConcepto `xml:"Retenciones>Retencion"`
}

type TrasladoConcepto struct {
	Base       string `xml:"Base,attr"`
	Impuesto   string `xml:"Impuesto,attr"`
	TipoFactor string `xml:"TipoFactor,attr"`
	TasaOCuota string `xml:"TasaOCuota,attr"`
	Importe    string `xml:"Importe,attr"`
}

type RetencionConcepto struct {
	Base       string `xml:"Base,attr"`
	Impuesto   string `xml:"Impuesto,attr"`
	TipoFactor string `xml:"TipoFactor,attr"`
	TasaOCuota string `xml:"TasaOCuota,attr"`
	Importe    string `xml:"Importe,attr"`
}

type ImpuestosGlobal struct {
	TotalImpuestosTrasladados string            `xml:"TotalImpuestosTrasladados,attr"`
	TotalImpuestosRetenidos   string            `xml:"TotalImpuestosRetenidos,attr"`
	Traslados                 []TrasladoGlobal  `xml:"Traslados>Traslado"`
	Retenciones               []RetencionGlobal `xml:"Retenciones>Retencion"`
}

type TrasladoGlobal struct {
	Base       string `xml:"Base,attr"`
	Impuesto   string `xml:"Impuesto,attr"`
	TipoFactor string `xml:"TipoFactor,attr"`
	TasaOCuota string `xml:"TasaOCuota,attr"`
	Importe    string `xml:"Importe,attr"`
}

type RetencionGlobal struct {
	Impuesto string `xml:"Impuesto,attr"`
	Importe  string `xml:"Importe,attr"`
}

func GenerarXML(data SaleData, certBase64, noCert string) (string, error) {
	fecha := data.Fecha.Format("2006-01-02T15:04:05")
	formaPago := data.FormaPago
	if formaPago == "" { formaPago = "01" }
	lugar := data.LugarExpedicion
	if lugar == "" { lugar = "64000" }

    cpReceptor := data.CodigoPostalReceptor
    if cpReceptor == "" { cpReceptor = lugar }

	var subtotal float64
	for _, item := range data.Items { subtotal += item.Subtotal }
	baseIVA := subtotal / 1.16
	ivaTotal := subtotal - baseIVA

	var conceptosXML []string
	for _, item := range data.Items {
		baseItem := item.Subtotal / 1.16
		ivaItem := item.Subtotal - baseItem
		concepto := fmt.Sprintf(
			`    <cfdi:Concepto ClaveProdServ="78101803" ClaveUnidad="E48" Cantidad="%.6f" Descripcion="%s" ValorUnitario="%.6f" Importe="%.6f" ObjetoImp="02">`+"\n"+
				`      <cfdi:Impuestos>`+"\n"+
				`        <cfdi:Traslados>`+"\n"+
				`          <cfdi:Traslado Base="%.2f" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="%.2f"/>`+"\n"+
				`        </cfdi:Traslados>`+"\n"+
				`      </cfdi:Impuestos>`+"\n"+
				`    </cfdi:Concepto>`,
			float64(item.Cantidad), escapeXML(item.Nombre),
			item.PrecioUnitario/1.16, baseItem, baseItem, ivaItem)
		conceptosXML = append(conceptosXML, concepto)
	}

	// ── Determinar tipo de receptor ──────────────────────────────────────────
	rfcReceptor := data.RFC
	esPublicoGeneral := rfcReceptor == "" || rfcReceptor == "XAXX010101000"

	var receptorXML, infoGlobalXML string

	if esPublicoGeneral {
		// Venta a público general — requiere InformacionGlobal
		meses := data.Fecha.Format("01")
		ano := data.Fecha.Year()
		infoGlobalXML = fmt.Sprintf(
			`  <cfdi:InformacionGlobal Periodicidad="04" Meses="%s" Año="%d"/>`, meses, ano)
		receptorXML = fmt.Sprintf(
			`  <cfdi:Receptor Rfc="XAXX010101000" Nombre="PUBLICO EN GENERAL" DomicilioFiscalReceptor="%s" RegimenFiscalReceptor="616" UsoCFDI="S01"/>`,
			lugar)
	} else {
		// Receptor identificado — sin InformacionGlobal
		nombreReceptor := data.NombreReceptor
		// Si el nombre no viene y el RFC es el del certificado de prueba, usar nombre SAT
		if nombreReceptor == "" && rfcReceptor == "EKU9003173C9" {
			nombreReceptor = "ESCUELA KEMPER URGATE"
		}
		if nombreReceptor == "" { nombreReceptor = rfcReceptor }
		infoGlobalXML = ""
		receptorXML = fmt.Sprintf(
			`  <cfdi:Receptor Rfc="%s" Nombre="%s" DomicilioFiscalReceptor="%s" RegimenFiscalReceptor="601" UsoCFDI="G03"/>`,
			rfcReceptor, escapeXML(nombreReceptor), cpReceptor)
	}

	// Construir XML
	var infoGlobalLine string
	if infoGlobalXML != "" {
		infoGlobalLine = infoGlobalXML + "\n"
	}

	xmlStr := fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
			`<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://www.sat.gob.mx/cfd/4 http://www.sat.gob.mx/sitio_internet/cfd/4/cfdv40.xsd" Version="4.0" Fecha="%s" Sello="" NoCertificado="%s" Certificado="%s" SubTotal="%.2f" Total="%.2f" Moneda="MXN" TipoDeComprobante="I" MetodoPago="PUE" FormaPago="%s" LugarExpedicion="%s" Exportacion="01">`+"\n"+
			`%s`+
			`  <cfdi:Emisor Rfc="EKU9003173C9" Nombre="ESCUELA KEMPER URGATE" RegimenFiscal="601"/>`+"\n"+
			`%s`+"\n"+
			`  <cfdi:Conceptos>`+"\n"+
			`%s`+"\n"+
			`  </cfdi:Conceptos>`+"\n"+
			`  <cfdi:Impuestos TotalImpuestosTrasladados="%.2f">`+"\n"+
			`    <cfdi:Traslados>`+"\n"+
			`      <cfdi:Traslado Base="%.2f" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="%.2f"/>`+"\n"+
			`    </cfdi:Traslados>`+"\n"+
			`  </cfdi:Impuestos>`+"\n"+
			`</cfdi:Comprobante>`,
		fecha, noCert, certBase64,
		baseIVA, subtotal, formaPago, lugar,
		infoGlobalLine,
		receptorXML,
		strings.Join(conceptosXML, "\n"),
		ivaTotal, baseIVA, ivaTotal)

	return xmlStr, nil
}

func FirmarXML(xmlStr string, keyBytes []byte, password string) (string, error) {
	privateKey, err := parsePrivateKey(keyBytes, password)
	if err != nil {
		return "", fmt.Errorf("parsear llave privada: %w", err)
	}
	cadena, err := generarCadenaOriginal(xmlStr)
	if err != nil {
		return "", fmt.Errorf("cadena original: %w", err)
	}
	hash := sha256.Sum256([]byte(cadena))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("firmar: %w", err)
	}
	sello := base64.StdEncoding.EncodeToString(signature)
	return strings.Replace(xmlStr, `Sello=""`, fmt.Sprintf(`Sello="%s"`, sello), 1), nil
}

func CertificadoBase64(certDER []byte) string {
	return base64.StdEncoding.EncodeToString(certDER)
}

func CadenaOriginalDebug(xmlStr string) (string, error) {
	return generarCadenaOriginal(xmlStr)
}

func parsePrivateKey(keyBytes []byte, password string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("no es PEM valido")
	}
	var derBytes []byte
	if x509.IsEncryptedPEMBlock(block) {
		var err error
		derBytes, err = x509.DecryptPEMBlock(block, []byte(password))
		if err != nil {
			return nil, fmt.Errorf("descifrar PEM: %w", err)
		}
	} else {
		derBytes = block.Bytes
	}
	key, err := x509.ParsePKCS8PrivateKey(derBytes)
	if err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok { return nil, fmt.Errorf("la llave no es RSA") }
		return rsaKey, nil
	}
	return x509.ParsePKCS1PrivateKey(derBytes)
}

func generarCadenaOriginal(xmlStr string) (string, error) {
	normalized := strings.ReplaceAll(xmlStr, "cfdi:", "")
	normalized = strings.ReplaceAll(normalized, `xmlns:cfdi="http://www.sat.gob.mx/cfd/4"`, "")
	normalized = strings.ReplaceAll(normalized, "AÃ±o=", "Ano=")
	normalized = strings.ReplaceAll(normalized, "Año=", "Ano=")

	var c Comprobante
	if err := xml.Unmarshal([]byte(normalized), &c); err != nil {
		return "", fmt.Errorf("parsear XML: %w", err)
	}

	var campos []string
	add := func(v string) {
		if v != "" { campos = append(campos, v) }
	}

	add(c.Version)
	add(c.Fecha)
	add(c.FormaPago)
	add(c.NoCertificado)
	add(c.SubTotal)
	add(c.Descuento)
	add(c.Moneda)
	add(c.TipoCambio)
	add(c.Total)
	add(c.TipoDeComprobante)
	add(c.Exportacion)
	add(c.MetodoPago)
	add(c.LugarExpedicion)
	add(c.Confirmacion)

	if c.InformacionGlobal != nil {
		add(c.InformacionGlobal.Periodicidad)
		add(c.InformacionGlobal.Meses)
		add(c.InformacionGlobal.Ano)
	}

	add(c.Emisor.Rfc)
	add(c.Emisor.Nombre)
	add(c.Emisor.RegimenFiscal)
	add(c.Emisor.FacAtrAdquirente)

	add(c.Receptor.Rfc)
	add(c.Receptor.Nombre)
	add(c.Receptor.DomicilioFiscalReceptor)
	add(c.Receptor.ResidenciaFiscal)
	add(c.Receptor.NumRegIdTrib)
	add(c.Receptor.RegimenFiscalReceptor)
	add(c.Receptor.UsoCFDI)

	for _, concepto := range c.Conceptos.Conceptos {
		add(concepto.ClaveProdServ)
		add(concepto.NoIdentificacion)
		add(concepto.Cantidad)
		add(concepto.ClaveUnidad)
		add(concepto.Unidad)
		add(concepto.Descripcion)
		add(concepto.ValorUnitario)
		add(concepto.Importe)
		add(concepto.Descuento)
		add(concepto.ObjetoImp)
		if concepto.Impuestos != nil {
			for _, t := range concepto.Impuestos.Traslados {
				add(t.Base); add(t.Impuesto); add(t.TipoFactor); add(t.TasaOCuota); add(t.Importe)
			}
			for _, r := range concepto.Impuestos.Retenciones {
				add(r.Base); add(r.Impuesto); add(r.TipoFactor); add(r.TasaOCuota); add(r.Importe)
			}
		}
	}

	if c.Impuestos != nil {
		for _, t := range c.Impuestos.Traslados {
			add(t.Base); add(t.Impuesto); add(t.TipoFactor); add(t.TasaOCuota); add(t.Importe)
		}
		add(c.Impuestos.TotalImpuestosRetenidos)
		add(c.Impuestos.TotalImpuestosTrasladados)
		for _, r := range c.Impuestos.Retenciones {
			add(r.Impuesto); add(r.Importe)
		}
	}

	return "||" + strings.Join(campos, "|") + "||", nil
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

