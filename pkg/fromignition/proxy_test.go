package fromignition

import "testing"

func TestParseDefaultEnvironmentProxy(t *testing.T) {
	conf := `[Manager]
DefaultEnvironment=HTTP_PROXY="http://p:8080"
DefaultEnvironment=HTTPS_PROXY="https://p:8080"
DefaultEnvironment=NO_PROXY=".cluster.local,10.0.0.0/8"
`
	p := parseDefaultEnvironmentProxy(conf)
	if p.HTTPProxy != "http://p:8080" || p.HTTPSProxy != "https://p:8080" || p.NoProxy != ".cluster.local,10.0.0.0/8" {
		t.Fatalf("unexpected proxy: %+v", p)
	}
}
