package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/turbopos/turbopos/services/cfdi/internal/finkok"
	"github.com/turbopos/turbopos/services/cfdi/internal/xmlgen"
)

func main() {
	username := os.Getenv("FINKOK_USER")
	password := os.Getenv("FINKOK_PASS")

	certBase64, noCert, err := finkok.LoadCertificate("services/cfdi/certs/test/eku9003173c9.cer")
	if err != nil {
		log.Fatalf("cert: %v", err)
	}

	data := xmlgen.SaleData{
		SaleID: "test-openssl-001",
		Fecha:  time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC),
		RFC:    "XAXX010101000",
		Items: []xmlgen.SaleItem{
			{Nombre: "Producto Test", Cantidad: 1, PrecioUnitario: 20.00, Subtotal: 20.00},
		},
		Total:           20.00,
		FormaPago:       "01",
		LugarExpedicion: "64000",
	}

	xmlStr, err := xmlgen.GenerarXML(data, certBase64, noCert)
	if err != nil {
		log.Fatalf("xml: %v", err)
	}

	// Leer sello generado por OpenSSL
	selloBytes, err := os.ReadFile(os.Getenv("TEMP") + "\\sello.b64")
	if err != nil {
		log.Fatalf("sello: %v", err)
	}
	sello := strings.TrimSpace(string(selloBytes))

	// Inyectar sello OpenSSL en el XML
	xmlConSello := strings.Replace(xmlStr, `Sello=""`, fmt.Sprintf(`Sello="%s"`, sello), 1)

	fmt.Println("✅ XML con sello OpenSSL listo")
	fmt.Printf("   Sello (primeros 40): %s...\n", sello[:40])

	// Enviar a Finkok
	fmt.Println("🚀 Enviando a Finkok con sello OpenSSL...")
	client := finkok.NewDemoClient(username, password)
	result, err := client.Timbrar(xmlConSello)
	if err != nil {
		log.Fatalf("SOAP: %v", err)
	}

	if result.Error != "" {
		fmt.Printf("❌ Error: %s\n", result.Error)
		return
	}

	fmt.Printf("\n🎉 UUID: %s\n", result.UUID)
	fmt.Printf("   Sello SAT: %s...\n", result.SelloSAT[:40])
	fmt.Printf("   Fecha: %s\n", result.FechaTimbrado)
}
