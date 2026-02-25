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

type SaleData struct {
    SaleID          string
    Fecha           time.Time
    RFC             string
    NombreReceptor  string
    Items           []SaleItem
    Total           float64
    FormaPago       string
    LugarExpedicion string
}

type SaleItem struct {
    Nombre         string
    Cantidad       int32
    PrecioUnitario float64
    Subtotal       float64
}

func GenerarXML(data SaleData, certBase64, noCert string) (string, error) {
    fecha := data.Fecha.Format("2006-01-02T15:04:05")
    formaPago := data.FormaPago
    if formaPago == "" { formaPago = "01" }
    lugar := data.LugarExpedicion
    if lugar == "" { lugar = "64000" }
    var subtotal float64
    for _, item := range data.Items { subtotal += item.Subtotal }
    baseIVA := subtotal / 1.16
    ivaTotal := subtotal - baseIVA
    var conceptosXML []string
    for _, item := range data.Items {
        baseItem := item.Subtotal / 1.16
        ivaItem := item.Subtotal - baseItem
        concepto := fmt.Sprintf("    <cfdi:Concepto ClaveProdServ=\"78101803\" ClaveUnidad=\"E48\" Cantidad=\"%.0f\" Descripcion=\"%s\" ValorUnitario=\"%.2f\" Importe=\"%.2f\" ObjetoImp=\"02\">\n      <cfdi:Impuestos>\n        <cfdi:Traslados>\n          <cfdi:Traslado Base=\"%.2f\" Impuesto=\"002\" TipoFactor=\"Tasa\" TasaOCuota=\"0.160000\" Importe=\"%.2f\"/>\n        </cfdi:Traslados>\n      </cfdi:Impuestos>\n    </cfdi:Concepto>",
            float64(item.Cantidad), escapeXML(item.Nombre), item.PrecioUnitario/1.16, baseItem, baseItem, ivaItem)
        conceptosXML = append(conceptosXML, concepto)
    }
    nombreReceptor := data.NombreReceptor
    if nombreReceptor == "" { nombreReceptor = "PUBLICO EN GENERAL" }
    rfcReceptor := data.RFC
    if rfcReceptor == "" || rfcReceptor == "XAXX010101000" {
        rfcReceptor = "XAXX010101000"
        nombreReceptor = "PUBLICO EN GENERAL"
    }
    xml := fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<cfdi:Comprobante\n  xmlns:cfdi=\"http://www.sat.gob.mx/cfd/4\"\n  xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"\n  xsi:schemaLocation=\"http://www.sat.gob.mx/cfd/4 http://www.sat.gob.mx/sitio_internet/cfd/4/cfdv40.xsd\"\n  Version=\"4.0\"\n  Fecha=\"%s\"\n  Sello=\"\"\n  NoCertificado=\"%s\"\n  Certificado=\"%s\"\n  SubTotal=\"%.2f\"\n  Total=\"%.2f\"\n  Moneda=\"MXN\"\n  TipoDeComprobante=\"I\"\n  MetodoPago=\"PUE\"\n  FormaPago=\"%s\"\n  LugarExpedicion=\"%s\"\n  Exportacion=\"01\">\n  <cfdi:Emisor Rfc=\"EKU9003173C9\" Nombre=\"EMPRESA PRUEBAS SA DE CV\" RegimenFiscal=\"601\"/>\n  <cfdi:Receptor\n    Rfc=\"%s\"\n    Nombre=\"%s\"\n    DomicilioFiscalReceptor=\"%s\"\n    RegimenFiscalReceptor=\"616\"\n    UsoCFDI=\"S01\"/>\n  <cfdi:Conceptos>\n%s\n  </cfdi:Conceptos>\n  <cfdi:Impuestos TotalImpuestosTrasladados=\"%.2f\">\n    <cfdi:Traslados>\n      <cfdi:Traslado Base=\"%.2f\" Impuesto=\"002\" TipoFactor=\"Tasa\" TasaOCuota=\"0.160000\" Importe=\"%.2f\"/>\n    </cfdi:Traslados>\n  </cfdi:Impuestos>\n</cfdi:Comprobante>",
        fecha, noCert, certBase64, baseIVA, subtotal, formaPago, lugar,
        rfcReceptor, nombreReceptor, lugar,
        strings.Join(conceptosXML, "\n"),
        ivaTotal, baseIVA, ivaTotal)
    return xml, nil
}

func FirmarXML(xmlStr string, keyBytes []byte, password string) (string, error) {
    privateKey, err := parsePrivateKey(keyBytes, password)
    if err != nil {
        return "", fmt.Errorf("parsear llave privada: %w", err)
    }
    cadena := generarCadenaOriginal(xmlStr)
    hash := sha256.Sum256([]byte(cadena))
    signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
    if err != nil {
        return "", fmt.Errorf("firmar: %w", err)
    }
    sello := base64.StdEncoding.EncodeToString(signature)
    return strings.Replace(xmlStr, "Sello=\"\"", fmt.Sprintf("Sello=\"%s\"", sello), 1), nil
}

func CertificadoBase64(certDER []byte) string {
    return base64.StdEncoding.EncodeToString(certDER)
}

func parsePrivateKey(keyBytes []byte, password string) (*rsa.PrivateKey, error) {
    block, _ := pem.Decode(keyBytes)
    if block == nil {
        return nil, fmt.Errorf("no es PEM valido — usa el archivo .pem convertido con openssl")
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

func generarCadenaOriginal(xmlStr string) string {
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