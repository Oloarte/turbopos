package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/turbopos/turbopos/services/cfdi/internal/finkok"
	"github.com/turbopos/turbopos/services/cfdi/internal/xmlgen"
)

func main() {
	username := os.Getenv("FINKOK_USER")
	password := os.Getenv("FINKOK_PASS")

	certBase64, noCert, _ := finkok.LoadCertificate("services/cfdi/certs/test/eku9003173c9.cer")

	data := xmlgen.SaleData{
		SaleID: "test-raw-debug",
		Fecha:  time.Now(),
		RFC:    "XAXX010101000",
		Items:  []xmlgen.SaleItem{{Nombre: "Test", Cantidad: 1, PrecioUnitario: 10.00, Subtotal: 10.00}},
		Total: 10.00, FormaPago: "01", LugarExpedicion: "64000",
	}
	xmlStr, _ := xmlgen.GenerarXML(data, certBase64, noCert)
	keyBytes, _ := os.ReadFile("services/cfdi/certs/test/eku9003173c9.pem")
	xmlFirmado, _ := xmlgen.FirmarXML(xmlStr, keyBytes, "12345678a")

	xmlBase64 := base64.StdEncoding.EncodeToString([]byte(xmlFirmado))
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
</soapenv:Envelope>`, xmlBase64, username, password)

	req, _ := http.NewRequest("POST", "https://demo-facturacion.finkok.com/servicios/soap/stamp.wsdl", bytes.NewBufferString(soapBody))
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://facturacion.finkok.com/stamp")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	fmt.Println("=== SOAP RESPONSE RAW ===")
	fmt.Println(string(body))
}
