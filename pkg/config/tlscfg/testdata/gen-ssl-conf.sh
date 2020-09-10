#!/usr/bin/env bash

# Generates the SSL conf files required for generating certificates.

domain_name="$1"
output_file="$2"

if [[ -z "$domain_name" || -z "$output_file" ]]; then
    printf "A script to generate SSL configuration files for testing purposes.\n\n"
    printf "Usage: ssl-conf-gen.sh DOMAIN_NAME OUTPUT_FILE\n\n"
    printf "Example: ssl-conf-gen.sh example.com ssl.conf\n"
    return 1
fi

cat << EOF > "$output_file"
[ req ]
prompt              = no
default_bits        = 2048
distinguished_name  = req_distinguished_name
req_extensions      = req_ext

[ req_distinguished_name ]
countryName         = AU
stateOrProvinceName = Australia
localityName        = Sydney
organizationName    = Logz.io
commonName          = Jaeger

[ req_ext ]
subjectAltName      = @alt_names

[alt_names]
DNS.1               = $domain_name
EOF
