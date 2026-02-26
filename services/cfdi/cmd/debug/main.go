package main

import (
    "fmt"
    "os"
    "time"
    "github.com/turbopos/turbopos/services/cfdi/internal/finkok"
    "github.com/turbopos/turbopos/services/cfdi/internal/xmlgen"
)

func main() {
    certBase64, noCert, _ := finkok.LoadCertificate("services/cfdi/certs/test/eku9003173c9.cer")
    data := xmlgen.SaleData{
        SaleID: "test-xslt",
        Fecha:  time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC),
        RFC:    "XAXX010101000",
        Items:  []xmlgen.SaleItem{{Nombre: "Producto Test", Cantidad: 1, PrecioUnitario: 20.00, Subtotal: 20.00}},
        Total: 20.00, FormaPago: "01", LugarExpedicion: "64000",
    }
    xmlStr, _ := xmlgen.GenerarXML(data, certBase64, noCert)
    os.WriteFile(`C:\Users\one\AppData\Local\Temp\test_cfdi.xml`, []byte(xmlStr), 0644)
    fmt.Println("XML guardado")

    cadena, _ := xmlgen.CadenaOriginalDebug(xmlStr)
    fmt.Println("CADENA:", cadena)
}