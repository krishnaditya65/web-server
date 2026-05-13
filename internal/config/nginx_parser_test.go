package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Lexer ─────────────────────────────────────────────────────────────────────

func TestLexer_BasicTokens(t *testing.T) {
	l := &lexer{src: `listen 8080;`}

	tok := l.next()
	assert.Equal(t, tokWord, tok.kind)
	assert.Equal(t, "listen", tok.val)

	tok = l.next()
	assert.Equal(t, tokWord, tok.kind)
	assert.Equal(t, "8080", tok.val)

	tok = l.next()
	assert.Equal(t, tokSemicolon, tok.kind)

	tok = l.next()
	assert.Equal(t, tokEOF, tok.kind)
}

func TestLexer_SkipsComments(t *testing.T) {
	l := &lexer{src: "# this is a comment\nlisten 8080;"}
	tok := l.next()
	assert.Equal(t, "listen", tok.val)
}

func TestLexer_QuotedString(t *testing.T) {
	l := &lexer{src: `"hello world"`}
	tok := l.next()
	assert.Equal(t, tokWord, tok.kind)
	assert.Equal(t, "hello world", tok.val)
}

func TestLexer_SingleQuotedString(t *testing.T) {
	l := &lexer{src: `'hello'`}
	tok := l.next()
	assert.Equal(t, "hello", tok.val)
}

func TestLexer_Braces(t *testing.T) {
	l := &lexer{src: "server { }"}

	assert.Equal(t, "server", l.next().val)
	assert.Equal(t, tokLBrace, l.next().kind)
	assert.Equal(t, tokRBrace, l.next().kind)
	assert.Equal(t, tokEOF, l.next().kind)
}

// ── Parser ────────────────────────────────────────────────────────────────────

func TestParser_SimpleDirective(t *testing.T) {
	nodes, err := parseBlock(&lexer{src: `listen 8080;`})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "listen", nodes[0].Directive)
	assert.Equal(t, []string{"8080"}, nodes[0].Args)
}

func TestParser_MultipleDirectives(t *testing.T) {
	src := `
		listen 8080;
		server_name example.com;
		gzip on;
	`
	nodes, err := parseBlock(&lexer{src: src})
	require.NoError(t, err)
	assert.Len(t, nodes, 3)
	assert.Equal(t, "listen", nodes[0].Directive)
	assert.Equal(t, "server_name", nodes[1].Directive)
	assert.Equal(t, "gzip", nodes[2].Directive)
}

func TestParser_Block(t *testing.T) {
	src := `
	server {
		listen 8080;
		gzip on;
	}
	`
	nodes, err := parseBlock(&lexer{src: src})
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	srv := nodes[0]
	assert.Equal(t, "server", srv.Directive)
	require.NotNil(t, srv.Block)
	assert.Len(t, srv.Block, 2)
	assert.Equal(t, "listen", srv.Block[0].Directive)
	assert.Equal(t, "gzip", srv.Block[1].Directive)
}

func TestParser_NestedBlocks(t *testing.T) {
	src := `
	server {
		location /users {
			proxy_pass http://backend;
		}
	}
	`
	nodes, err := parseBlock(&lexer{src: src})
	require.NoError(t, err)

	serverBlock := nodes[0].Block
	require.Len(t, serverBlock, 1)
	assert.Equal(t, "location", serverBlock[0].Directive)
	assert.Equal(t, []string{"/users"}, serverBlock[0].Args)
}

func TestParser_EmptyBlock(t *testing.T) {
	nodes, err := parseBlock(&lexer{src: `upstream empty {}`})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Empty(t, nodes[0].Block)
}

func TestParser_DirectiveWithMultipleArgs(t *testing.T) {
	nodes, err := parseBlock(&lexer{src: `listen 8443 ssl;`})
	require.NoError(t, err)
	assert.Equal(t, []string{"8443", "ssl"}, nodes[0].Args)
}

// ── Translator ────────────────────────────────────────────────────────────────

func TestTranslator_ServerPorts(t *testing.T) {
	src := `
	server {
		listen 8080;
		listen 8443 ssl;
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, err := FromNginx(nodes)
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 8443, cfg.Server.HTTPSPort)
}

func TestTranslator_TLSPaths(t *testing.T) {
	src := `
	server {
		ssl_certificate     /etc/certs/cert.pem;
		ssl_certificate_key /etc/certs/key.pem;
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	assert.Equal(t, "/etc/certs/cert.pem", cfg.Server.TLSCertFile)
	assert.Equal(t, "/etc/certs/key.pem", cfg.Server.TLSKeyFile)
}

func TestTranslator_GzipDirectives(t *testing.T) {
	src := `
	server {
		gzip on;
		gzip_min_length 512;
		gzip_comp_level 4;
		gzip_types text/plain application/json;
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	assert.True(t, cfg.Gzip.Enabled)
	assert.Equal(t, 512, cfg.Gzip.MinLength)
	assert.Equal(t, 4, cfg.Gzip.Level)
	assert.Contains(t, cfg.Gzip.ContentTypes, "text/plain")
	assert.Contains(t, cfg.Gzip.ContentTypes, "application/json")
}

func TestTranslator_RateLimit(t *testing.T) {
	src := `
	server {
		limit_req_zone $binary_remote_addr zone=api:10m rate=30r/s;
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	assert.Equal(t, float64(30), cfg.Rate.RequestsPerSecond)
}

func TestTranslator_UpstreamGroup(t *testing.T) {
	src := `
	upstream backend {
		server localhost:9001 weight=3;
		server localhost:9002 weight=1;
	}
	server {
		location / {
			proxy_pass http://backend;
		}
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, err := FromNginx(nodes)
	require.NoError(t, err)

	require.Len(t, cfg.Proxy.Routes, 1)
	require.Len(t, cfg.Proxy.Routes[0].Upstreams, 2)
	assert.Equal(t, "http://localhost:9001", cfg.Proxy.Routes[0].Upstreams[0].URL)
	assert.Equal(t, 3, cfg.Proxy.Routes[0].Upstreams[0].Weight)
	assert.Equal(t, 1, cfg.Proxy.Routes[0].Upstreams[1].Weight)
}

func TestTranslator_LeastConnAlgorithm(t *testing.T) {
	src := `
	upstream backend {
		server localhost:9001;
		least_conn;
	}
	server {
		location / { proxy_pass http://backend; }
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	assert.Equal(t, "least_conn", cfg.Proxy.Algorithm)
}

func TestTranslator_LocationTimeout(t *testing.T) {
	src := `
	server {
		location /api {
			proxy_pass http://localhost:9001;
			proxy_read_timeout 10s;
		}
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	require.Len(t, cfg.Proxy.Routes, 1)
	assert.Equal(t, 10, cfg.Proxy.Routes[0].TimeoutSeconds)
}

func TestTranslator_BodySizePerLocation(t *testing.T) {
	src := `
	server {
		client_max_body_size 10m;
		location /upload {
			proxy_pass http://localhost:9001;
			client_max_body_size 50m;
		}
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	require.Len(t, cfg.Proxy.Routes, 1)
	// Location-level overrides server-level.
	assert.Equal(t, int64(50*1024*1024), cfg.Proxy.Routes[0].MaxBodyBytes)
}

func TestTranslator_LimitReqAddsRateLimitPlugin(t *testing.T) {
	src := `
	server {
		limit_req_zone $binary_remote_addr zone=api:10m rate=20r/s;
		location / {
			proxy_pass http://localhost:9001;
			limit_req zone=api burst=40;
		}
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	require.Len(t, cfg.Proxy.Routes, 1)
	plugins := cfg.Proxy.Routes[0].Plugins
	require.NotEmpty(t, plugins)
	assert.Equal(t, "rate-limit", plugins[0].Name)
	assert.True(t, plugins[0].Enabled)
	assert.Equal(t, 40, plugins[0].Config["burst"])
}

func TestTranslator_UnknownDirectiveSkipped(t *testing.T) {
	src := `
	server {
		error_page 404 /404.html;
		try_files $uri $uri/ =404;
		location / {
			proxy_pass http://localhost:9001;
		}
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, err := FromNginx(nodes)

	require.NoError(t, err, "unknown directives must not cause errors")
	require.Len(t, cfg.Proxy.Routes, 1)
}

func TestTranslator_DirectProxyPass(t *testing.T) {
	src := `
	server {
		location /health {
			proxy_pass http://localhost:9001;
		}
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	require.Len(t, cfg.Proxy.Routes, 1)
	require.Len(t, cfg.Proxy.Routes[0].Upstreams, 1)
	assert.Equal(t, "http://localhost:9001", cfg.Proxy.Routes[0].Upstreams[0].URL)
}

func TestTranslator_LocationWithoutProxyPassSkipped(t *testing.T) {
	src := `
	server {
		location /static { root /var/www; }
		location /api { proxy_pass http://localhost:9001; }
	}
	`
	nodes, _ := parseBlock(&lexer{src: src})
	cfg, _ := FromNginx(nodes)

	// Only /api should be a route (static has no proxy_pass).
	require.Len(t, cfg.Proxy.Routes, 1)
	assert.Equal(t, "/api", cfg.Proxy.Routes[0].PathPrefix)
}

func TestParseRate_VariousFormats(t *testing.T) {
	cases := []struct {
		args     []string
		expected float64
	}{
		{[]string{"zone=api:10m", "rate=20r/s"}, 20},
		{[]string{"rate=60r/m"}, 1},  // 60 per minute = 1 per second
		{[]string{"rate=5/s"}, 5},
		{[]string{}, 0},
	}

	for _, tc := range cases {
		got := parseRate(tc.args)
		assert.InDelta(t, tc.expected, got, 0.01)
	}
}
