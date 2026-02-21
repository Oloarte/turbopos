
### go vet

```
go : # github.com/turbopos/turbopos/services/cfdi/cmd
En línea: 16 Carácter: 35
+ Append-Section "go vet"         { go vet ./... }
+                                   ~~~~~~~~~~~~
    + CategoryInfo          : NotSpecified: (# github.com/tu...rvices/cfdi/cmd:String) [], R 
   emoteException
    + FullyQualifiedErrorId : NativeCommandError
 
# [github.com/turbopos/turbopos/services/cfdi/cmd]
vet.exe: services\cfdi\cmd\main.go:99:5: declared and not used: lis

```

### staticcheck

```
-: # github.com/turbopos/turbopos/services/cfdi/cmd
services\cfdi\cmd\main.go:99:5: declared and not used: lis (compile)
gen\go\auth\v1\auth.pb.go:173:3: file_auth_v1_auth_proto_msgTypes[0].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\auth\v1\auth.pb.go:185:3: file_auth_v1_auth_proto_msgTypes[1].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\cfdi\v1\cfdi.pb.go:230:3: file_cfdi_v1_cfdi_proto_msgTypes[0].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\cfdi\v1\cfdi.pb.go:242:3: file_cfdi_v1_cfdi_proto_msgTypes[1].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\proto\auth\v1\auth.pb.go:173:3: file_proto_auth_v1_auth_proto_msgTypes[0].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\proto\auth\v1\auth.pb.go:185:3: file_proto_auth_v1_auth_proto_msgTypes[1].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\proto\cfdi\v1\cfdi.pb.go:221:3: file_proto_cfdi_v1_cfdi_proto_msgTypes[0].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
gen\go\proto\cfdi\v1\cfdi.pb.go:233:3: file_proto_cfdi_v1_cfdi_proto_msgTypes[1].Exporter is deprecated: Exporter will be removed the next time we bump protoimpl.GenVersion. See https://github.com/golang/protobuf/issues/1640  (SA1019)
services\bff\cmd\main.go:29:20: grpc.Dial is deprecated: use NewClient instead.  Will be supported throughout 1.x.  (SA1019)
services\bff\cmd\main.go:30:20: grpc.Dial is deprecated: use NewClient instead.  Will be supported throughout 1.x.  (SA1019)

```

### gosec

```
gosec : [gosec] 2026/02/21 04:09:32 Including rules: default
En línea: 18 Carácter: 35
+ Append-Section "gosec"          { gosec ./... }
+                                   ~~~~~~~~~~~
    + CategoryInfo          : NotSpecified: ([gosec] 2026/02... rules: default:String) [], R 
   emoteException
    + FullyQualifiedErrorId : NativeCommandError
 
[gosec] 2026/02/21 04:09:32 Excluding rules: default
[gosec] 2026/02/21 04:09:32 Including analyzers: default
[gosec] 2026/02/21 04:09:32 Excluding analyzers: default
[gosec] 2026/02/21 04:09:32 Import directory: C:\dev\turbopos\gen\go\auth\v1
[gosec] 2026/02/21 04:09:32 Import directory: C:\dev\turbopos\services\audit\cmd
[gosec] 2026/02/21 04:09:32 Import directory: C:\dev\turbopos\gen\go\proto\cfdi\v1
[gosec] 2026/02/21 04:09:32 Import directory: C:\dev\turbopos\services\auth\cmd\server
[gosec] 2026/02/21 04:09:36 Checking package: main
[gosec] 2026/02/21 04:09:36 Checking file: C:\dev\turbopos\services\audit\cmd\main.go
[gosec] 2026/02/21 04:09:36 Import directory: C:\dev\turbopos\services\cfdi\cmd
[gosec] 2026/02/21 04:09:37 Checking package: cfdiv1
[gosec] 2026/02/21 04:09:37 Checking file: C:\dev\turbopos\gen\go\proto\cfdi\v1\cfdi.pb.go
[gosec] 2026/02/21 04:09:37 Checking file: 
C:\dev\turbopos\gen\go\proto\cfdi\v1\cfdi_grpc.pb.go
[gosec] 2026/02/21 04:09:37 Checking package: authv1
[gosec] 2026/02/21 04:09:37 Checking file: C:\dev\turbopos\gen\go\auth\v1\auth.pb.go
[gosec] 2026/02/21 04:09:37 Checking file: C:\dev\turbopos\gen\go\auth\v1\auth_grpc.pb.go
[gosec] 2026/02/21 04:09:37 Checking package: main
[gosec] 2026/02/21 04:09:37 Checking file: C:\dev\turbopos\services\auth\cmd\server\main.go
[gosec] 2026/02/21 04:09:37 Import directory: C:\dev\turbopos\services\marketing\cmd
[gosec] 2026/02/21 04:09:37 Import directory: C:\dev\turbopos\services\reportbot\cmd
[gosec] 2026/02/21 04:09:37 Import directory: C:\dev\turbopos\gen\go\cfdi\v1
[gosec] 2026/02/21 04:09:38 Checking package: main
[gosec] 2026/02/21 04:09:38 Checking file: C:\dev\turbopos\services\marketing\cmd\main.go
[gosec] 2026/02/21 04:09:38 Import directory: C:\dev\turbopos\gen\go\proto\auth\v1
[gosec] 2026/02/21 04:09:40 Checking package: main
[gosec] 2026/02/21 04:09:40 Checking file: C:\dev\turbopos\services\reportbot\cmd\main.go
[gosec] 2026/02/21 04:09:40 Import directory: C:\dev\turbopos\services\auth\cmd
[gosec] 2026/02/21 04:09:41 Checking package: cfdiv1
[gosec] 2026/02/21 04:09:41 Checking file: C:\dev\turbopos\gen\go\cfdi\v1\cfdi.pb.go
[gosec] 2026/02/21 04:09:41 Checking file: C:\dev\turbopos\gen\go\cfdi\v1\cfdi_grpc.pb.go
[gosec] 2026/02/21 04:09:41 Import directory: C:\dev\turbopos\services\bff\cmd
[gosec] 2026/02/21 04:09:41 Checking package: authv1
[gosec] 2026/02/21 04:09:41 Checking file: C:\dev\turbopos\gen\go\proto\auth\v1\auth.pb.go
[gosec] 2026/02/21 04:09:41 Checking file: 
C:\dev\turbopos\gen\go\proto\auth\v1\auth_grpc.pb.go
[gosec] 2026/02/21 04:09:42 Import directory: C:\dev\turbopos\services\security\cmd
[gosec] 2026/02/21 04:09:43 Checking package: main
[gosec] 2026/02/21 04:09:43 Checking file: C:\dev\turbopos\services\auth\cmd\main.go
[gosec] 2026/02/21 04:09:43 Checking package: main
[gosec] 2026/02/21 04:09:43 Checking file: C:\dev\turbopos\services\security\cmd\scanner.go
[gosec] 2026/02/21 04:09:44 Checking package: main
[gosec] 2026/02/21 04:09:44 Checking file: C:\dev\turbopos\services\bff\cmd\main.go
Results:

Golang errors in file: [services\cfdi\cmd]:

  > [line 0 : column 0] - parsing errors in pkg "main": parsing line: strconv.Atoi: parsing "\\dev\\turbopos\\services\\cfdi\\cmd\\main.go": invalid syntax



[[30;43mC:\dev\turbopos\services\bff\cmd\main.go:54[0m] - G114 (CWE-676): Use of net/http serve function that has no support for setting timeouts (Confidence: HIGH, Severity: MEDIUM)
    53: 
  > 54:     log.Fatal(http.ListenAndServe(":8080", corsHandler))
    55: }

Autofix: 

[[30;43mC:\dev\turbopos\services\security\cmd\scanner.go:27[0m] - G304 (CWE-22): Potential file inclusion via variable (Confidence: HIGH, Severity: MEDIUM)
    26:         }
  > 27:         content, _ := os.ReadFile(path)
    28:         for name, re := range sensitivePatterns {

Autofix: Consider using os.Root to scope file access under a fixed root (Go >=1.24). Prefer root.Open/root.Stat over os.Open/os.Stat to prevent directory traversal.

[[30;43mC:\dev\turbopos\services\auth\cmd\main.go:55[0m] - G102 (CWE-200): Binds to all network interfaces (Confidence: HIGH, Severity: MEDIUM)
    54: 
  > 55:     lis, err := net.Listen("tcp", Port)
    56:     if err != nil {

Autofix: 

[[37;40mC:\dev\turbopos\services\security\cmd\scanner.go:20-32[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    19:     root, _ := filepath.Abs("../../../")
  > 20:     filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
  > 21:         if err != nil || d.IsDir() { return nil }
  > 22:         if d.Name() == "scanner.go" || strings.Contains(path, ".git") { return nil }
  > 23:         ext := strings.ToLower(filepath.Ext(path))
  > 24:         if ext == ".key" || ext == ".cer" {
  > 25:             log.Printf("? CRITICAL: %s", path); findings++; return nil
  > 26:         }
  > 27:         content, _ := os.ReadFile(path)
  > 28:         for name, re := range sensitivePatterns {
  > 29:             if re.Match(content) { log.Printf("??  %s en: %s", name, path); findings++ }
  > 30:         }
  > 31:         return nil
  > 32:     })
    33:     if findings > 0 { os.Exit(1) }

Autofix: 

[[37;40mC:\dev\turbopos\services\bff\cmd\main.go:89[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    88:     }
  > 89:     json.NewEncoder(w).Encode(logs)
    90: }

Autofix: 

[[37;40mC:\dev\turbopos\services\bff\cmd\main.go:86[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    85:         var et, aid, cat string
  > 86:         rows.Scan(&et, &aid, &cat)
    87:         logs = append(logs, map[string]string{"msg": "Venta " + aid + " (" + et + ")", "time": cat, "type": "info"})

Autofix: 

[[37;40mC:\dev\turbopos\services\bff\cmd\main.go:76[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    75: func (gw *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
  > 76:     json.NewEncoder(w).Encode(map[string]interface{}{"system": "ONLINE", "services": map[string]string{"auth": "OK", "cfdi": "OK", "db": "CONNECTED"}})
    77: }

Autofix: 

[[37;40mC:\dev\turbopos\services\bff\cmd\main.go:72[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    71:     }
  > 72:     json.NewEncoder(w).Encode(res)
    73: }

Autofix: 

[[37;40mC:\dev\turbopos\services\bff\cmd\main.go:70[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    69:     if err != nil {
  > 70:         w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); return
    71:     }

Autofix: 

[[37;40mC:\dev\turbopos\services\bff\cmd\main.go:63[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    62:     }
  > 63:     json.NewDecoder(r.Body).Decode(&req)
    64:     ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)

Autofix: 

[[37;40mC:\dev\turbopos\services\audit\cmd\main.go:43[0m] - G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
    42:         var e Event
  > 43:         rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.CreatedAt)
    44:         log.Printf("?? [AUDIT-LOG] Venta: %s | Evento: %s | Fecha: %s | ESTADO: Verificado para SAT", 

Autofix: 

[1;36mSummary:[0m
  Gosec  : dev
  Files  : 15
  Lines  : 1790
  Nosec  : 0
  Issues : [1;31m11[0m


```

### govulncheck

```
govulncheck : govulncheck: loading packages: 
En línea: 19 Carácter: 35
+ Append-Section "govulncheck"    { govulncheck ./... }
+                                   ~~~~~~~~~~~~~~~~~
    + CategoryInfo          : NotSpecified: (govulncheck: loading packages: :String) [], Rem 
   oteException
    + FullyQualifiedErrorId : NativeCommandError
 
There are errors with the provided package patterns:

C:\Users\one\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.24.0.windows-amd64\src\slices\iter.go
:50:17: cannot range over seq (variable of type iter.Seq[E])
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\internal\timeseries\timeseries.go:6:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.24.0.windows-amd64\src\maps\iter.go:5
1:20: cannot range over seq (variable of type iter.Seq2[K, V])
C:\Users\one\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.24.0.windows-amd64\src\text\template\
exec.go:405:18: cannot range over val.Seq() (value of type iter.Seq[reflect.Value])
C:\Users\one\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.24.0.windows-amd64\src\text\template\
exec.go:460:19: cannot range over val.Seq() (value of type iter.Seq[reflect.Value])
C:\Users\one\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.24.0.windows-amd64\src\text\template\
exec.go:473:22: cannot range over val.Seq2() (value of type iter.Seq2[reflect.Value, 
reflect.Value])
C:\Users\one\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.24.0.windows-amd64\src\crypto\x509\ve
rify.go:1476:20: cannot range over pg.parents() (value of type iter.Seq[*policyGraphNode])
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\trace\events.go:5:1: package requires newer 
Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\backoff\backoff.go:25:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\grpclog\internal\grpclog.go:20:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\grpclog\component.go:19:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\connectivity\connectivity.go:21:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\attributes\attributes.go:26:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\credentials\credentials.go:17
:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\envconfig\envconfig.go:20:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\detrand\rand.go:10:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\errors\errors.go:6:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\encoding\protowire\wire.go:10:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\pragma\pragma.go:7:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\reflect\protoreflect\methods.go:5
:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\encoding\messageset\mess
ageset.go:6:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\flags\flags.go:6:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\genid\any_gen.go:7:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\order\order.go:5:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\strs\strings.go:6:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\reflect\protoregistry\registry.go
:16:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\runtime\protoiface\legacy.go:5:1:
 package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\proto\checkinit.go:5:1: package 
requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\credentials\credentials.go:23:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\sys@v0.38.0\windows\aliases.go:7:1: package requires 
newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\serviceconfig\serviceconfig.go:26:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\experimental.go:18:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\channelz\channel.go:19:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\channelz\channelz.go:30:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\metadata\metadata.go:22:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\stats\handlers.go:19:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\experimental\stats\metricregistry.go:1
9:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\resolver\map.go:19:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\balancer.go:21:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\base\balancer.go:19:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\pickfirst\internal\internal.g
o:19:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\grpclog\prefix_logger.go:21:1
: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\encoding\json\decode.go:
5:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\descfmt\stringer.go:6:1:
 package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\descopts\options.go:10:1
: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\editiondefaults\defaults
.go:7:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\encoding\text\decode.go:
5:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\encoding\defval\default.
go:10:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\filedesc\build.go:9:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\set\ints.go:6:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\encoding\protojson\decode.go:5:1:
 package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\encoding\prototext\decode.go:5:1:
 package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\encoding\tag\tag.go:7:1:
 package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\protolazy\bufferreader.g
o:7:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\impl\api_export.go:5:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\filetype\build.go:7:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\version\version.go:6:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\runtime\protoimpl\impl.go:12:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\protoadapt\convert.go:6:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\pretty\pretty.go:20:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\pickfirst\pickfirst.go:21:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\endpointsharding\endpointshar
ding.go:26:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\roundrobin\roundrobin.go:22:1
: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\codes\code_string.go:19:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\credentials\insecure\insecure.go:21:1:
 package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\encoding\internal\internal.go:20:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\grpcutil\compressor.go:19:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\mem\buffer_pool.go:19:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\encoding\encoding.go:26:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\encoding\proto\proto.go:21:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\backoff\backoff.go:23:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\balancer\gracefulswitch\confi
g.go:19:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\balancerload\load.go:19:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\types\known\durationpb\duration.p
b.go:74:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\types\known\timestamppb\timestamp
.pb.go:73:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\binarylog\grpc_binarylog_v1\binarylog.
pb.go:25:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\types\known\anypb\any.pb.go:115:1
: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\genproto\googleapis\rpc@v0.0.0-20251029180050-ab938
6a59fda\status\status.pb.go:21:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\status\status.go:28:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\status\status.go:28:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\binarylog\binarylog.go:21:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\buffer\unbounded.go:19:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\grpcsync\callback_serializer.
go:19:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\idle\idle.go:21:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\metadata\metadata.go:22:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\serviceconfig\duration.go:19:
1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\resolver\config_selector.go:2
0:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\proxyattributes\proxyattribut
es.go:21:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\text@v0.31.0\transform\transform.go:9:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\text@v0.31.0\unicode\bidi\bidi.go:13:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\text@v0.31.0\secure\bidirule\bidirule.go:9:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\text@v0.31.0\unicode\norm\composition.go:5:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\idna\go118.go:9:1: package requires newer 
Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\http\httpguts\guts.go:10:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\http2\hpack\encode.go:5:1: package requires 
newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\internal\httpcommon\ascii.go:5:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\golang.org\x\net@v0.47.0\http2\ascii.go:5:1: package requires newer 
Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\stats\labels.go:20:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\syscall\syscall_nonlinux.go:2
4:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\transport\networktype\network
type.go:21:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\keepalive\keepalive.go:21:1: package 
requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\peer\peer.go:21:1: package requires 
newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\tap\tap.go:26:1: package requires 
newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\transport\bdp_estimator.go:19
:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\resolver\delegatingresolver\d
elegatingresolver.go:21:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\resolver\passthrough\passthro
ugh.go:21:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\resolver\unix\unix.go:20:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\balancer\grpclb\state\state.go:21:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\resolver\dns\internal\interna
l.go:20:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\internal\resolver\dns\dns_resolver.go:
21:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\resolver\dns\dns_resolver.go:21:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\backoff.go:22:1: package requires 
newer Go version go1.24
C:\dev\turbopos\gen\go\auth\v1\auth.pb.go:7:1: package requires newer Go version go1.24
C:\dev\turbopos\gen\go\cfdi\v1\cfdi.pb.go:7:1: package requires newer Go version go1.24
C:\dev\turbopos\gen\go\proto\auth\v1\auth.pb.go:7:1: package requires newer Go version go1.24
C:\dev\turbopos\gen\go\proto\cfdi\v1\cfdi.pb.go:7:1: package requires newer Go version go1.24
C:\dev\turbopos\services\audit\cmd\main.go:1:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\reflection\grpc_reflection_v1\reflecti
on.pb.go:28:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\reflection\grpc_reflection_v1alpha\ref
lection.pb.go:25:1: package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\types\descriptorpb\descriptor.pb.
go:42:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\internal\editionssupport\editions
.go:6:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\types\gofeaturespb\go_features.pb
.go:11:1: package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\protobuf@v1.36.11\reflect\protodesc\desc.go:13:1: 
package requires newer Go version go1.23
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\reflection\internal\internal.go:22:1: 
package requires newer Go version go1.24
C:\Users\one\go\pkg\mod\google.golang.org\grpc@v1.78.0\reflection\adapt.go:19:1: package 
requires newer Go version go1.24
C:\dev\turbopos\services\auth\cmd\main.go:1:4: package requires newer Go version go1.24
C:\dev\turbopos\services\auth\cmd\server\main.go:1:4: package requires newer Go version 
go1.24
C:\dev\turbopos\services\bff\cmd\main.go:2:1: package requires newer Go version go1.24
C:\dev\turbopos\services\cfdi\cmd\main.go:1:4: package requires newer Go version go1.24
C:\dev\turbopos\services\marketing\cmd\main.go:1:1: package requires newer Go version go1.24
C:\dev\turbopos\services\reportbot\cmd\main.go:1:1: package requires newer Go version go1.24
C:\dev\turbopos\services\security\cmd\scanner.go:1:1: package requires newer Go version 
go1.24

For details on package patterns, see 
https://pkg.go.dev/cmd/go#hdr-Package_lists_and_patterns.


```

### go list (updates)

```
github.com/turbopos/turbopos
cel.dev/expr v0.24.0 [v0.25.1]
cloud.google.com/go/compute/metadata v0.9.0
github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.30.0 [v1.31.0]
github.com/cespare/xxhash/v2 v2.3.0
github.com/cncf/xds/go v0.0.0-20251022180443-0feb69152e9f [v0.0.0-20260202195803-dba9d589def2]
github.com/envoyproxy/go-control-plane v0.13.5-0.20251024222203-75eaa193e329 [v0.14.0]
github.com/envoyproxy/go-control-plane/envoy v1.35.0 [v1.37.0]
github.com/envoyproxy/go-control-plane/ratelimit v0.1.0
github.com/envoyproxy/protoc-gen-validate v1.2.1 [v1.3.3]
github.com/go-jose/go-jose/v4 v4.1.3
github.com/go-logr/logr v1.4.3
github.com/go-logr/stdr v1.2.2
github.com/golang/glog v1.2.5
github.com/golang/protobuf v1.5.4 (deprecated)
github.com/google/go-cmp v0.7.0
github.com/google/uuid v1.6.0
github.com/lib/pq v1.11.2
github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
github.com/spiffe/go-spiffe/v2 v2.6.0
go.opentelemetry.io/auto/sdk v1.2.1
go.opentelemetry.io/contrib/detectors/gcp v1.38.0 [v1.40.0]
go.opentelemetry.io/otel v1.38.0 [v1.40.0]
go.opentelemetry.io/otel/metric v1.38.0 [v1.40.0]
go.opentelemetry.io/otel/sdk v1.38.0 [v1.40.0]
go.opentelemetry.io/otel/sdk/metric v1.38.0 [v1.40.0]
go.opentelemetry.io/otel/trace v1.38.0 [v1.40.0]
golang.org/x/crypto v0.44.0 [v0.48.0]
golang.org/x/mod v0.29.0 [v0.33.0]
golang.org/x/net v0.47.0 [v0.50.0]
golang.org/x/oauth2 v0.32.0 [v0.35.0]
golang.org/x/sync v0.18.0 [v0.19.0]
golang.org/x/sys v0.38.0 [v0.41.0]
golang.org/x/term v0.37.0 [v0.40.0]
golang.org/x/text v0.31.0 [v0.34.0]
golang.org/x/tools v0.38.0 [v0.42.0]
gonum.org/v1/gonum v0.16.0 [v0.17.0]
google.golang.org/genproto/googleapis/api v0.0.0-20251029180050-ab9386a59fda [v0.0.0-20260217215200-42d3e9bedb6d]
google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda [v0.0.0-20260217215200-42d3e9bedb6d]
google.golang.org/grpc v1.78.0 [v1.79.1]
google.golang.org/protobuf v1.36.11

```

### Syft SBOM

```
syft : [0000]  WARN no explicit name and version provided for directory source, deriving 
artifact ID from the given path (which is not ideal)
En línea: 30 Carácter: 38
+ Append-Section "Syft SBOM"         { syft dir:. | Select-Object -Firs ...
+                                      ~~~~~~~~~~
    + CategoryInfo          : NotSpecified: ([0000]  WARN no...h is not ideal):String) [], R 
   emoteException
    + FullyQualifiedErrorId : NativeCommandError
 
NAME                                       VERSION                             TYPE           
actions/checkout                           v4                                  github-action  
actions/setup-go                           v5                                  github-action  
bufbuild/buf-setup-action                  v1                                  github-action  
github.com/lib/pq                          v1.11.2                             go-module      
golang.org/x/net                           v0.47.0                             go-module      
golang.org/x/sys                           v0.38.0                             go-module      
golang.org/x/text                          v0.31.0                             go-module      
google.golang.org/genproto/googleapis/rpc  v0.0.0-20251029180050-ab9386a59fda  go-module      
google.golang.org/grpc                     v1.78.0                             go-module      
google.golang.org/protobuf                 v1.36.11                            go-module

```

### Trivy FS scan

```
trivy : 2026-02-21T04:09:57-06:00	INFO	[vuln] Vulnerability scanning is enabled
En línea: 31 Carácter: 38
+ ... scan"     { trivy fs --severity HIGH,CRITICAL --scanners vuln --no-pr ...
+                 ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
    + CategoryInfo          : NotSpecified: (2026-02-21T04:0...ning is enabled:String) [], R 
   emoteException
    + FullyQualifiedErrorId : NativeCommandError
 
2026-02-21T04:10:00-06:00	INFO	Number of language-specific files	num=1
2026-02-21T04:10:00-06:00	INFO	[gomod] Detecting vulnerabilities...

Report Summary

ÔöîÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔö¼ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔö¼ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÉ
Ôöé Target Ôöé Type  Ôöé Vulnerabilities Ôöé
Ôö£ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔö╝ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔö╝ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöñ
Ôöé go.mod Ôöé gomod Ôöé        0        Ôöé
ÔööÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔö┤ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔö┤ÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÇÔöÿ
Legend:
- '-': Not scanned
- '0': Clean (no security findings detected)


```
