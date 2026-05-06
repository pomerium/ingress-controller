package certificate_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pomerium/ingress-controller/internal/certificate"
	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func TestGetNamesFromConfig(t *testing.T) {
	t.Parallel()

	amazonData, err := os.ReadFile("testdata/amazon.pem")
	require.NoError(t, err)

	googleData, err := os.ReadFile("testdata/google.pem")
	require.NoError(t, err)

	certificateNames, routeNames := certificate.GetNamesFromConfig(
		[]*configpb.KeyPair{{Certificate: amazonData}},
		[]*configpb.Route{
			{From: "https://www.example.com"},
			{From: "https://127.0.0.1"},
			{From: "http://insecure.example.com"},
			{From: "ssh://ssh.example.com"},
			{From: "https://*.wildcard.example.com"},
			{From: "https://api.example.com"},
			{},
			{From: "https://api.example.com:8443"},
			{From: "<NOT A URL>"},
		},
		[]*configpb.Settings{{Certificates: []*configpb.Settings_Certificate{{CertBytes: googleData}}}},
	)
	assert.Equal(t, []string{
		"*.2mdn-cn.net",
		"*.aa.peg.a2z.com",
		"*.ab.peg.a2z.com",
		"*.ac.peg.a2z.com",
		"*.admob-cn.com",
		"*.aistudio.google.com",
		"*.ampproject.net.cn",
		"*.ampproject.org.cn",
		"*.android.com",
		"*.android.google.cn",
		"*.app-measurement-cn.com",
		"*.appengine.google.com",
		"*.bdn.dev",
		"*.bz.peg.a2z.com",
		"*.chrome.google.cn",
		"*.cloud.google.com",
		"*.crowdsource.google.com",
		"*.dartsearch-cn.net",
		"*.datacompute.google.com",
		"*.developers.google.cn",
		"*.doubleclick-cn.net",
		"*.doubleclick.cn",
		"*.flash.android.com",
		"*.fls.doubleclick-cn.net",
		"*.fls.doubleclick.cn",
		"*.g.cn",
		"*.g.co",
		"*.g.doubleclick-cn.net",
		"*.g.doubleclick.cn",
		"*.gcp.gvt2.com",
		"*.gcpcdn.gvt1.com",
		"*.gemini.cloud.google.com",
		"*.ggpht.cn",
		"*.gkecnapps.cn",
		"*.google-analytics-cn.com",
		"*.google-analytics.com",
		"*.google.ca",
		"*.google.cl",
		"*.google.co.in",
		"*.google.co.jp",
		"*.google.co.uk",
		"*.google.com",
		"*.google.com.ar",
		"*.google.com.au",
		"*.google.com.br",
		"*.google.com.co",
		"*.google.com.mx",
		"*.google.com.tr",
		"*.google.com.vn",
		"*.google.de",
		"*.google.es",
		"*.google.fr",
		"*.google.hu",
		"*.google.it",
		"*.google.nl",
		"*.google.pl",
		"*.google.pt",
		"*.googleadservices-cn.com",
		"*.googleapis-cn.com",
		"*.googleapis.cn",
		"*.googleapps-cn.com",
		"*.googlecnapps.cn",
		"*.googlecommerce.com",
		"*.googledownloads.cn",
		"*.googleflights-cn.net",
		"*.googleoptimize-cn.com",
		"*.googlesandbox-cn.com",
		"*.googlesyndication-cn.com",
		"*.googletagmanager-cn.com",
		"*.googletagservices-cn.com",
		"*.googletraveladservices-cn.com",
		"*.googlevads-cn.com",
		"*.gstatic-cn.com",
		"*.gstatic.cn",
		"*.gstatic.com",
		"*.gvt1-cn.com",
		"*.gvt1.com",
		"*.gvt2-cn.com",
		"*.gvt2.com",
		"*.metric.gstatic.com",
		"*.music.youtube.com",
		"*.origin-test.bdn.dev",
		"*.peg.a2z.com",
		"*.recaptcha-cn.net",
		"*.recaptcha.net.cn",
		"*.safeframe.googlesyndication-cn.com",
		"*.safenup.googlesandbox-cn.com",
		"*.urchin.com",
		"*.url.google.com",
		"*.widevine.cn",
		"*.youtube-nocookie.com",
		"*.youtube.com",
		"*.youtubeeducation.com",
		"*.youtubekids.com",
		"*.yt.be",
		"*.ytimg.com",
		"2mdn-cn.net",
		"WR2",
		"admob-cn.com",
		"ai.android",
		"amazon.co.jp",
		"amazon.co.uk",
		"amazon.com",
		"amazon.com.au",
		"amazon.de",
		"amazon.jp",
		"ampproject.net.cn",
		"ampproject.org.cn",
		"amzn.com",
		"android.clients.google.com",
		"android.com",
		"app-measurement-cn.com",
		"buckeye-retail-website.amazon.com",
		"buybox.amazon.com",
		"corporate.amazon.com",
		"dartsearch-cn.net",
		"doubleclick-cn.net",
		"doubleclick.cn",
		"edgeflow-dp.aero.04f01a85e-frontier.amazon.com.au",
		"edgeflow-dp.aero.47cf2c8c9-frontier.amazon.com",
		"edgeflow-dp.aero.4d5ad1d2b-frontier.amazon.co.jp",
		"edgeflow-dp.aero.abe2c2f23-frontier.amazon.de",
		"edgeflow-dp.aero.bfbdc3ca1-frontier.amazon.co.uk",
		"edgeflow.aero.04f01a85e-frontier.amazon.com.au",
		"edgeflow.aero.47cf2c8c9-frontier.amazon.com",
		"edgeflow.aero.4d5ad1d2b-frontier.amazon.co.jp",
		"edgeflow.aero.abe2c2f23-frontier.amazon.de",
		"edgeflow.aero.bfbdc3ca1-frontier.amazon.co.uk",
		"g.cn",
		"g.co",
		"ggpht.cn",
		"gkecnapps.cn",
		"goo.gl",
		"google-analytics-cn.com",
		"google-analytics.com",
		"google.com",
		"googleadservices-cn.com",
		"googleapis-cn.com",
		"googleapps-cn.com",
		"googlecnapps.cn",
		"googlecommerce.com",
		"googledownloads.cn",
		"googleflights-cn.net",
		"googleoptimize-cn.com",
		"googlesandbox-cn.com",
		"googlesyndication-cn.com",
		"googletagmanager-cn.com",
		"googletagservices-cn.com",
		"googletraveladservices-cn.com",
		"googlevads-cn.com",
		"gvt1-cn.com",
		"gvt2-cn.com",
		"home.amazon.com",
		"huddles.amazon.com",
		"iphone.amazon.com",
		"music.youtube.com",
		"origin-www.amazon.co.jp",
		"origin-www.amazon.co.uk",
		"origin-www.amazon.com",
		"origin-www.amazon.com.au",
		"origin-www.amazon.de",
		"origin2-www.amazon.co.jp",
		"origin2-www.amazon.com",
		"recaptcha-cn.net",
		"recaptcha.net.cn",
		"shop.business.amazon.com",
		"uedata.amazon.co.uk",
		"uedata.amazon.com",
		"urchin.com",
		"us.amazon.com",
		"widevine.cn",
		"www.amazon.co.jp",
		"www.amazon.co.uk",
		"www.amazon.com",
		"www.amazon.com.au",
		"www.amazon.de",
		"www.amazon.jp",
		"www.amzn.com",
		"www.goo.gl",
		"youtu.be",
		"youtube.com",
		"youtubeeducation.com",
		"youtubekids.com",
		"yp.amazon.com",
		"yt.be",
	}, certificateNames)
	assert.Equal(t, []string{
		"api.example.com",
		"www.example.com",
	}, routeNames)
}

func TestIterateServerCertificatesFromPEM(t *testing.T) {
	t.Parallel()

	amazonData, err := os.ReadFile("testdata/amazon.pem")
	require.NoError(t, err)

	googleData, err := os.ReadFile("testdata/google.pem")
	require.NoError(t, err)

	var serialNumbers []string
	for cert := range certificate.IterateServerCertificatesFromPEM(amazonData) {
		serialNumbers = append(serialNumbers, cert.SerialNumber.String())
	}
	for cert := range certificate.IterateServerCertificatesFromPEM(googleData) {
		serialNumbers = append(serialNumbers, cert.SerialNumber.String())
	}
	assert.Equal(t, []string{
		"17254315543325975286547466867573710433",
		"186718756357656129965259402690993979140",
		"170058220837755766831192027518741805976",
	}, serialNumbers)
}
