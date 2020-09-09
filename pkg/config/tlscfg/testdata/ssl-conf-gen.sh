#!/usr/bin/env bash

domain_name="$1"
output_file="$2"

if [[ -z "$domain_name" || -z "$output_file" ]]; then
    printf "A script to generate SSL configuration files for testing purposes.\n\n"
    printf "Usage: ssl-conf-gen.sh DOMAIN_NAME OUTPUT_FILE\n\n"
    printf "Example: ssl-conf-gen.sh example.com ssl.conf\n"
    exit 1
fi

cat << EOF > "$output_file"
[ req ]
default_bits       = 2048
distinguished_name = req_distinguished_name
req_extensions     = req_ext

[ req_distinguished_name ]
countryName                 = Country Name (2 letter code)
countryName_default         = AU
stateOrProvinceName         = State or Province Name (full name)
stateOrProvinceName_default = Australia
localityName                = Locality Name (eg, city)
localityName_default        = Sydney
organizationName            = Organization Name (eg, company)
organizationName_default    = Logz.io
commonName                  = Common Name (e.g. server FQDN or YOUR name)
commonName_max              = 64
commonName_default          = jaeger

[ req_ext ]
subjectAltName = @alt_names

[alt_names]
DNS.1   = $domain_name
EOF
