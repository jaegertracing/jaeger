package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

func extractSectionFromFile(filePath, startMarker, endMarker string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}
	text := string(data)

	startIndex := strings.Index(text, startMarker)
	if startIndex == -1 {
		return "", fmt.Errorf("start marker %q not found", startMarker)
	}
	startIndex += len(startMarker)

	endIndex := strings.Index(text, endMarker)
	if endIndex == -1 {
		return "", fmt.Errorf("end marker %q not found", endMarker)
	}

	backendSection := text[startIndex:endIndex]
	return backendSection, nil
}

func extractAfterStart(filePath, startMarker string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}
	text := string(data)

	startIndex := strings.Index(text, startMarker)
	if startIndex == -1 {
		return "", fmt.Errorf("start marker %q not found", startMarker)
	}
	startIndex += len(startMarker)

	endIndex := len(text)-1

	backendSection := text[startIndex:endIndex]
	return backendSection, nil
}

func main() {
	backendFileName := "RELEASE.md"
	backendStartMarker := "# Jaeger Backend Release Process"
	backendEndMarker := "## Patch Release"
	backendSection, err := extractSectionFromFile(backendFileName, backendStartMarker, backendEndMarker)
	if err != nil {
		log.Fatalf("Failed to extract backendSection: %v", err)
	}
	re_star := regexp.MustCompile(`(\n\s*)(\*)(\s)`)
	backendSection = re_star.ReplaceAllString(backendSection, "$1$2 [ ]$3")
	re_num :=regexp.MustCompile(`(\n\s*)([0-9]*\.)(\s)`)
	backendSection = re_num.ReplaceAllString(backendSection, "$1* [ ]$3")

	docFilename:= "DOC_RELEASE.md"
	docStartMarker := "# Release instructions"
	docEndMarker := "### Auto-generated documentation for CLI flags"
	docSection, err := extractSectionFromFile(docFilename, docStartMarker, docEndMarker)
	if err != nil{
		log.Fatalf("Failed to extract documentation section: %v", err)
		
	}
	re_dash :=regexp.MustCompile(`(\n\s*)(\-)`)
	docSection=re_dash.ReplaceAllString(docSection, "$1* [ ]")
	
	uiFilename := "jaeger-ui/RELEASE.md"
	uiStartMarker := "# Cutting a Jaeger UI release"
	uiSection, err := extractAfterStart(uiFilename, uiStartMarker)
	if err != nil{
		log.Fatalf("Failed to extract UI section: %v", err)
	}
	uiSection = re_dash.ReplaceAllString(uiSection, "$1$2 [ ]$3")
	uiSection = re_num.ReplaceAllString(uiSection, "$1* [ ]$3")
	


	fmt.Println("# UI Release")
	fmt.Println(uiSection)
	fmt.Println("# Backend Release")
	fmt.Println(backendSection)
	fmt.Println("# Doc Release")
	fmt.Println(docSection)

}
