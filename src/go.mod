module github.com/Belphemur/obsidian-headless/src-go

go 1.26

replace github.com/1password/onepassword-sdk-go => ./internal/1passwordstub

require (
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/byteness/keyring v1.9.1
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/djherbis/times v1.6.0
	github.com/fsnotify/fsnotify v1.10.0
	github.com/golang-migrate/migrate/v4 v4.19.1
	github.com/gorilla/websocket v1.5.3
	github.com/jedisct1/go-aes-siv v1.0.0
	github.com/rs/zerolog v1.35.1
	github.com/sergi/go-diff v1.4.0
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.50.0
	golang.org/x/sys v0.43.0
	golang.org/x/term v0.42.0
	golang.org/x/text v0.36.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.50.0
)

require (
	github.com/1Password/connect-sdk-go v1.5.4-0.20250417152128-c154b387248b // indirect
	github.com/1password/onepassword-sdk-go v0.4.1-beta.1 // indirect
	github.com/byteness/go-keychain v0.0.0-20260108220220-c96c38f7f906 // indirect
	github.com/byteness/go-libsecret v0.0.0-20260108215642-107379d3dee0 // indirect
	github.com/byteness/percent v0.2.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/dvsekhvalnov/jose2go v1.8.0 // indirect
	github.com/dylibso/observe-sdk/go v0.0.0-20240828172851-9145d8ad07e1 // indirect
	github.com/extism/go-sdk v1.7.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20251118225945-96ee0021ea0f // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.21 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/noamcohen97/touchid-go v0.3.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pelletier/go-toml/v2 v2.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tetratelabs/wabin v0.0.0-20230304001439-f6f874872834 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	modernc.org/libc v1.72.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
