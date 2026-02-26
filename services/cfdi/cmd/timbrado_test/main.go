package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/turbopos/turbopos/services/cfdi/internal/finkok"
	"github.com/turbopos/turbopos/services/cfdi/internal/xmlgen"
)

func main() {
	username := os.Getenv("FINKOK_USER")
	password := os.Getenv("FINKOK_PASS")
	if username == "" {
		log.Fatal("Falta FINKOK_USER")
	}
	if password == "" {
		log.Fatal("Falta FINKOK_PASS")
	}

	certBase64, noCert, err := finkok.LoadCertificate("services/cfdi/certs/test/eku9003173c9.cer")
	if err != nil {
		log.Fatalf("Error cargando certificado: %v", err)
	}
	fmt.Printf("✅ Certificado cargado — NoCert: %s\n", noCert)

	data := xmlgen.SaleData{
		SaleID: "test-timbrado-001",
		Fecha:  time.Now(),
		RFC:    "XAXX010101000",
		Items: []xmlgen.SaleItem{
			{Nombre: "Coca-Cola 600ml", Cantidad: 1, PrecioUnitario: 20.00, Subtotal: 20.00},
			{Nombre: "Papas Sabritas", Cantidad: 1, PrecioUnitario: 18.00, Subtotal: 18.00},
		},
		Total:           38.00,
		FormaPago:       "01",
		LugarExpedicion: "64000",
	}

	xmlStr, err := xmlgen.GenerarXML(data, certBase64, noCert)
	if err != nil {
		log.Fatalf("Error generando XML: %v", err)
	}
	fmt.Println("✅ XML CFDI 4.0 generado")

	keyBytes, err := os.ReadFile("services/cfdi/certs/test/eku9003173c9.pem")
	if err != nil {
		log.Fatalf("Error leyendo .key: %v", err)
	}

	xmlFirmado, err := xmlgen.FirmarXML(xmlStr, keyBytes, "12345678a")
	if err != nil {
		log.Fatalf("Error firmando XML: %v", err)
	}
	fmt.Println("✅ XML firmado con CSD")

	fmt.Println("\n🚀 Enviando a Finkok para timbrado real...")
	client := finkok.NewDemoClient(username, password)
	result, err := client.Timbrar(xmlFirmado)
	if err != nil {
		log.Fatalf("Error de conexion con Finkok: %v", err)
	}

	if result.Error != "" {
		fmt.Printf("\n❌ Error de Finkok: %s\n", result.Error)
		for _, inc := range result.Incidencias {
			fmt.Printf("   Codigo: %s — %s\n", inc.CodigoError, inc.MensajeIncidencia)
		}
		return
	}

	fmt.Println("\n🎉 ¡TIMBRADO EXITOSO!")
	fmt.Printf("   UUID SAT:     %s\n", result.UUID)
	if len(result.SelloSAT) > 40 {
		fmt.Printf("   Sello SAT:    %s...\n", result.SelloSAT[:40])
	}
	fmt.Printf("   NoCert SAT:   %s\n", result.NoCertSAT)
	fmt.Printf("   Fecha Timbre: %s\n", result.FechaTimbrado)

	// Debug: imprimir primeros 500 chars del XML timbrado para ver estructura TFD
	if result.UUID == "" && result.XML != "" {
		fmt.Println("\n=== DEBUG XML TIMBRADO (primeros 800 chars) ===")
		xmlDebug := result.XML
		if len(xmlDebug) > 800 {
			xmlDebug = xmlDebug[:800]
		}
		fmt.Println(xmlDebug)
	}
}
