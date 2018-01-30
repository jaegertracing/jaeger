package proxy_if

import (
  "testing"
  "github.com/jaegertracing/jaeger/plugin/storage/authorizingproxy/proxy_if"
)

func TestValidBaggage(t *testing.T) {
  proxyIf := proxy_if.NewProxyIf("baggage.x-test-key==test-value")
  if !proxyIf.IsValid() {
    t.Error("Expected the input to be valid.")
  }
  if !proxyIf.IsBaggage() {
    t.Error("Expected the input to be a baggage.")
  }
  if proxyIf.Key() != "x-test-key" {
    t.Error("Invalid key.")
  }
  if proxyIf.Value() != "test-value" {
    t.Error("Invalid value.")
  }
}

func TestValidTag(t *testing.T) {
  proxyIf := proxy_if.NewProxyIf("tag.x-test-key==test-value")
  if !proxyIf.IsValid() {
    t.Error("Expected the input to be valid.")
  }
  if !proxyIf.IsTag() {
    t.Error("Expected the input to be a tag.")
  }
  if proxyIf.Key() != "x-test-key" {
    t.Error("Invalid key.")
  }
  if proxyIf.Value() != "test-value" {
    t.Error("Invalid value.")
  }
}

func TestInvalidInputType(t *testing.T) {
  proxyIf := proxy_if.NewProxyIf("invalid-type.x-test-key==value")
  if proxyIf.IsValid() {
    t.Error("Expected the input to be invalid.")
  }
}

func TestIncompleteInput(t *testing.T) {
  proxyIf := proxy_if.NewProxyIf("baggage.x-test-key=")
  if proxyIf.IsValid() {
    t.Error("Expected the input to be invalid.")
  }
}