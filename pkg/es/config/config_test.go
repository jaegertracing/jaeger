package config

import "testing"

func TestNewClient(t *testing.T){
	tests := []struct{
		name             string
        config           *Configuration
        expectedError    bool
	}{
		
	}
	for _, test := range tests{
		t.Run(test.name,func(t *testing.T) {
			
		})
	}
}

//suucess state - noserver - 
//only one of c.Password c.PasswordFilePath should be given - 
//if c.PasswordFilePath != "" -> filepath should be there
//c.LogLevel should be one of debug, info, error

//this func can't find test for it GetHTTPRoundTripper
//explore internal/metricstest 