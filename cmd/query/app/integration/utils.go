package integration

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)
/* based on: https://github.com/openshift/oauth-proxy/blob/master/test/e2e/proxy_test.go */

func submitOAuthForm(client *http.Client, response *http.Response, user string) (*http.Response, error) {
	responseBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	responseBuffer := bytes.NewBuffer(responseBytes)

	body, err := html.Parse(responseBuffer)
	if err != nil {
		return nil, err
	}

	forms := getElementsByTagName(body, "form")
	if len(forms) != 1 {
		errMsg := "expected OpenShift form"
		return nil, fmt.Errorf(errMsg)
	}

	formReq, err := newRequestFromForm(forms[0], response.Request.URL, user)
	if err != nil {
		return nil, err
	}

	postResp, err := client.Do(formReq)
	if err != nil {
		return nil, err
	}

	return postResp, nil
}

func visit(n *html.Node, visitor func(*html.Node)) {
	visitor(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visit(c, visitor)
	}
}

func getElementsByTagName(root *html.Node, tagName string) []*html.Node {
	elements := []*html.Node{}
	visit(root, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tagName {
			elements = append(elements, n)
		}
	})
	return elements
}

func getAttr(element *html.Node, attrName string) (string, bool) {
	for _, attr := range element.Attr {
		if attr.Key == attrName {
			return attr.Val, true
		}
	}
	return "", false
}

func newRequestFromForm(form *html.Node, currentURL *url.URL, user string) (*http.Request, error) {
	var (
		reqMethod string
		reqURL    *url.URL
		reqBody   io.Reader
		reqHeader = http.Header{}
		err       error
	)

	// Method defaults to GET if empty
	if method, _ := getAttr(form, "method"); len(method) > 0 {
		reqMethod = strings.ToUpper(method)
	} else {
		reqMethod = "GET"
	}

	// URL defaults to current URL if empty
	action, _ := getAttr(form, "action")
	reqURL, err = currentURL.Parse(action)
	if err != nil {
		return nil, err
	}

	formData := url.Values{}
	if reqMethod == "GET" {
		// Start with any existing query params when we're submitting via GET
		formData = reqURL.Query()
	}
	addedSubmit := false
	for _, input := range getElementsByTagName(form, "input") {
		if name, ok := getAttr(input, "name"); ok {
			if value, ok := getAttr(input, "value"); ok {
				inputType, _ := getAttr(input, "type")

				switch inputType {
				case "text":
					if name == "username" {
						formData.Add(name, user)
					}
				case "password":
					if name == "password" {
						formData.Add(name, "foo")
					}
				case "submit":
					// If this is a submit input, only add the value of the first one.
					// We're simulating submitting the form.
					if !addedSubmit {
						formData.Add(name, value)
						addedSubmit = true
					}
				case "radio", "checkbox":
					if _, checked := getAttr(input, "checked"); checked {
						formData.Add(name, value)
					}
				default:
					formData.Add(name, value)
				}
			}
		}
	}

	switch reqMethod {
	case "GET":
		reqURL.RawQuery = formData.Encode()
	case "POST":
		reqHeader.Set("Content-Type", "application/x-www-form-urlencoded")
		reqBody = strings.NewReader(formData.Encode())
	default:
		return nil, fmt.Errorf("unknown method: %s", reqMethod)
	}

	req, err := http.NewRequest(reqMethod, reqURL.String(), reqBody)
	if err != nil {
		return nil, err
	}

	req.Header = reqHeader
	return req, nil
}

func getResponse(host string, client *http.Client) (*http.Response, error) {
	req, err := http.NewRequest("GET", host, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func closeResponseBody(resp *http.Response)  {
	if resp == nil {
		return
	}
	err := resp.Body.Close()
	if err != nil {
		log.Print(err)
	}
}

func newOauthAuthenticatedClient(route string, user string) (*http.Client, error) {
	host := "https://" + route + "/oauth/start"
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{Transport: tr, Jar: jar}
	startUrl := host
	resp, err := getResponse(startUrl, client)
	if err != nil {
		return client, err
	}
	defer closeResponseBody(resp)
	// OpenShift login
	loginResp, err := submitOAuthForm(client, resp, user)
	if err != nil {
		return client,err
	}
	defer closeResponseBody(loginResp)

	// authorization grant form
	grantResp, err := submitOAuthForm(client, loginResp, user)
	/*if err != nil {
		return client, err
	}*/
	defer closeResponseBody(grantResp)
	return client, nil
}
