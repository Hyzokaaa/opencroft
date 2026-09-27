package nginx

import "testing"

// Modelled on the output of `nginx -T` from a real Debian host: server blocks
// live in their own files at depth zero, while the default site sits nested
// inside the http block of nginx.conf.
const realDump = `# configuration file /etc/nginx/nginx.conf:
user www-data;
events {
	worker_connections 768;
}
http {
	# server_names_hash_bucket_size 64;
	# server_name_in_redirect off;
	include /etc/nginx/croft.d/*.conf;

	server {
		listen 8081;
		server_name _;
		location / {
			return 404;
		}
	}
}

# configuration file /etc/nginx/sites-enabled/openhelpdesk:
server {
	listen 80;
	server_name dev.openhelpdesk.dev;
	return 301 https://$host$request_uri;
}

server {
	listen 443 ssl;
	server_name dev.openhelpdesk.dev;

	ssl_certificate /etc/letsencrypt/live/dev.openhelpdesk.dev/fullchain.pem;
	ssl_certificate_key /etc/letsencrypt/live/dev.openhelpdesk.dev/privkey.pem;

	location / {
		proxy_pass http://10.210.68.50;
	}
}

server {
	listen 80;
	server_name openhelpdesk.network;

	location / {
		proxy_pass http://10.210.68.50;
	}
}
`

func TestScanFindsEveryServerBlock(t *testing.T) {
	blocks := scanServerBlocks(realDump)

	if len(blocks) != 4 {
		t.Fatalf("expected 4 server blocks, got %d", len(blocks))
	}

	// A block nested inside http still belongs to the file it was printed under.
	if blocks[0].File != "/etc/nginx/nginx.conf" {
		t.Errorf("nested block came from %q", blocks[0].File)
	}
	if blocks[1].File != "/etc/nginx/sites-enabled/openhelpdesk" {
		t.Errorf("top-level block came from %q", blocks[1].File)
	}
}

func TestLocationBlocksDoNotEndTheServer(t *testing.T) {
	blocks := scanServerBlocks(realDump)

	// The https block must survive its location block and keep the proxy_pass.
	target, port := upstream(blocks[2].Body)
	if target != "10.210.68.50" || port != 80 {
		t.Fatalf("expected 10.210.68.50:80, got %s:%d", target, port)
	}
}

func TestCatchAllServerNameIsNotADomain(t *testing.T) {
	names := serverNames(scanServerBlocks(realDump)[0].Body)
	if len(names) != 0 {
		t.Errorf("`_` should not be reported as a domain, got %v", names)
	}
}

func TestTLSIsDetectedOnlyWhereDeclared(t *testing.T) {
	blocks := scanServerBlocks(realDump)

	if servesTLS(blocks[1].Body) {
		t.Error("the redirect block does not serve TLS")
	}
	if !servesTLS(blocks[2].Body) {
		t.Error("the https block does serve TLS")
	}
}

func TestRedirectBlockHasNoUpstream(t *testing.T) {
	target, _ := upstream(scanServerBlocks(realDump)[1].Body)
	if target != "" {
		t.Errorf("a redirect has no upstream, got %q", target)
	}
}

func TestExplicitPortIsRead(t *testing.T) {
	blocks := scanServerBlocks("# configuration file /x:\nserver {\n\tserver_name a.example.com;\n\tlocation / {\n\t\tproxy_pass http://10.0.0.5:3000;\n\t}\n}\n")

	target, port := upstream(blocks[0].Body)
	if target != "10.0.0.5" || port != 3000 {
		t.Fatalf("expected 10.0.0.5:3000, got %s:%d", target, port)
	}
}

// The shape an install script leaves inside a container: the distribution's
// catch-all, and one site that serves a build and passes /api/ on. Only the
// second is a site somebody put there.
const insideAContainer = `# configuration file /etc/nginx/sites-enabled/default:
server {
	listen 80 default_server;
	root /var/www/html;
	server_name _;
	location / {
		try_files $uri $uri/ =404;
	}
}
# configuration file /etc/nginx/sites-enabled/openhelpdesk.conf:
server {
    listen 80;
    server_name dev.openhelpdesk.dev;

    root /var/www/openhelpdesk/;
    # root /var/www/old;

    location / {
        try_files $uri $uri/ /index.html;
    }
    location /api/ {
        proxy_pass http://localhost:3000/;
    }
}
`

func TestAStaticSiteIsTheDirectoryANamedServerServes(t *testing.T) {
	sites := StaticSites(insideAContainer)

	if len(sites) != 1 {
		t.Fatalf("found %d sites: %+v", len(sites), sites)
	}
	if sites[0].Root != "/var/www/openhelpdesk" {
		t.Errorf("root is %q", sites[0].Root)
	}
	if len(sites[0].Names) != 1 || sites[0].Names[0] != "dev.openhelpdesk.dev" {
		t.Errorf("names are %v", sites[0].Names)
	}
}

// A block that only passes requests on serves no directory of its own.
func TestAProxyIsNotAStaticSite(t *testing.T) {
	dump := "server {\n    server_name api.example.com;\n    location / { proxy_pass http://10.0.0.2:3000; }\n}\n"
	if sites := StaticSites(dump); len(sites) != 0 {
		t.Errorf("found %+v", sites)
	}
}
