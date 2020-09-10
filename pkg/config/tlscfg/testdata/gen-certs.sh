#!/usr/bin/env bash

# The following commands were used to create the CA, server and client's certificates and keys in this directory used by unit tests.
# These certificates use the Subject Alternative Name extension rather than the Common Name, which will be unsupported in Go 1.15.

usage() {
  echo "Usage: $0 [-d]"
  echo
  echo "-d  Dry-run mode. PEM files will not be modified."
  exit 1
}

dry_run=false

while getopts "d" o; do
    case "${o}" in
        d)
            dry_run=true
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND-1))

set -ex

# Create temp dir for generated files.
tmp_dir=$(mktemp -d -t certificates)
clean_up() {
    ARG=$?
    if [ $dry_run = true ]; then
      echo "Dry-run complete. Generated files can be found in $tmp_dir"
    else
      rm -rf "$tmp_dir"
    fi
    exit $ARG
}
trap clean_up EXIT

gen_ssl_conf() {
  domain_name=$1
  output_file=$2

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
}

# Generate config files.
# The server name (under alt_names in the ssl.conf) is `example.com`. (in accordance to [RFC 2006](https://tools.ietf.org/html/rfc2606))
gen_ssl_conf example.com "$tmp_dir/ssl.conf"
gen_ssl_conf wrong.com "$tmp_dir/wrong-ssl.conf"

# Create CA (accept defaults from prompts).
openssl genrsa -out "$tmp_dir/example-CA-key.pem"  2048
openssl req -new -key "$tmp_dir/example-CA-key.pem" -x509 -days 3650 -out "$tmp_dir/example-CA-cert.pem" -config "$tmp_dir/ssl.conf"

# Create Wrong CA (a dummy CA which doesn't provide any certificate; accept defaults from prompts).
openssl genrsa -out "$tmp_dir/wrong-CA-key.pem" 2048
openssl req -new -key "$tmp_dir/wrong-CA-key.pem" -x509 -days 3650 -out "$tmp_dir/wrong-CA-cert.pem" -config "$tmp_dir/wrong-ssl.conf"

# Create client and server keys.
openssl genrsa -out "$tmp_dir/example-server-key.pem" 2048
openssl genrsa -out "$tmp_dir/example-client-key.pem" 2048

# Create certificate sign request using the above created keys and configuration given and commandline arguments.
openssl req -new -nodes -key "$tmp_dir/example-server-key.pem" -out "$tmp_dir/example-server.csr" -config "$tmp_dir/ssl.conf"
openssl req -new -nodes -key "$tmp_dir/example-client-key.pem" -out "$tmp_dir/example-client.csr" -config "$tmp_dir/ssl.conf"

# Creating the client and server certificate.
openssl x509 -req \
             -sha256 \
             -days 3650 \
             -in "$tmp_dir/example-server.csr" \
             -signkey "$tmp_dir/example-server-key.pem" \
             -out "$tmp_dir/example-server-cert.pem" \
             -extensions req_ext \
             -CA "$tmp_dir/example-CA-cert.pem" \
             -CAkey "$tmp_dir/example-CA-key.pem" \
             -CAcreateserial \
             -extfile "$tmp_dir/ssl.conf"
openssl x509 -req \
             -sha256 \
             -days 3650 \
             -in "$tmp_dir/example-client.csr" \
             -signkey "$tmp_dir/example-client-key.pem" \
             -out "$tmp_dir/example-client-cert.pem" \
             -extensions req_ext \
             -CA "$tmp_dir/example-CA-cert.pem" \
             -CAkey "$tmp_dir/example-CA-key.pem" \
             -CAcreateserial \
             -extfile "$tmp_dir/ssl.conf"

# Copy PEM files.
if [ $dry_run = false ]; then
  cp "$tmp_dir/example-CA-cert.pem" \
     "$tmp_dir/example-client-cert.pem" \
     "$tmp_dir/example-client-key.pem" \
     "$tmp_dir/example-server-cert.pem" \
     "$tmp_dir/example-server-key.pem" \
     "$tmp_dir/wrong-CA-cert.pem" .
fi