package main
import (
    "fmt"
    "io/fs"
    "log"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)
var sensitivePatterns = map[string]*regexp.Regexp{
    "SAT_PRIVATE_KEY": regexp.MustCompile(`(?i)BEGIN (RSA|EC|ENCRYPTED) PRIVATE KEY`),
    "GENERIC_SECRET":  regexp.MustCompile(`(?i)(password|secret|apikey|token|passwd|auth_key)\s*[:=]\s*["'][^"']+["']`),
    "PG_CONN_STRING":  regexp.MustCompile(`postgres://[^:]+:[^@]+@[^/]+/[^?]+`),
}
func main() {
    log.Println("???  SecOps-Scanner v1.1 (Autoinmune)")
    findings := 0
    root, _ := filepath.Abs("../../../")
    filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() { return nil }
        if d.Name() == "scanner.go" || strings.Contains(path, ".git") { return nil }
        ext := strings.ToLower(filepath.Ext(path))
        if ext == ".key" || ext == ".cer" {
            log.Printf("? CRITICAL: %s", path); findings++; return nil
        }
        content, _ := os.ReadFile(path)
        for name, re := range sensitivePatterns {
            if re.Match(content) { log.Printf("??  %s en: %s", name, path); findings++ }
        }
        return nil
    })
    if findings > 0 { os.Exit(1) }
    fmt.Println("? ESCANEO LIMPIO.")
}
