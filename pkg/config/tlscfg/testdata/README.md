# Example Certificate Authority and Certificate creation for testing

The following commands were used to create the CA, server and client's certificates and keys. These certificates use the Subject Alternative Name extension rather than the Common Name, which will be unsupported in Go 1.15.

```bash
# generate config files
./ssl-conf-gen.sh example.com ssl.conf
./ssl-conf-gen.sh wrong.com wrong-ssl.conf

# create CA (accept defaults from prompts)
openssl genrsa -out example-CA-key.pem  2048
openssl req -new -key example-CA-key.pem -x509 -days 3650 -out example-CA-cert.pem -config ssl.conf

# create Wrong CA (a dummy CA which doesn't provide any certificate; accept defaults from prompts)
openssl genrsa -out wrong-CA-key.pem  2048
openssl req -new -key wrong-CA-key.pem -x509 -days 3650 -out wrong-CA-cert.pem -config wrong-ssl.conf

# create client and server keys
openssl genrsa -out example-server-key.pem 2048
openssl genrsa -out example-client-key.pem 2048

# create certificate sign request using the above created keys and configuration given and commandline arguments.
# (accept defaults from prompts)
openssl req -new -nodes -key example-server-key.pem -out example-server.csr -config ssl.conf
openssl req -new -nodes -key example-client-key.pem -out example-client.csr -config ssl.conf

# creating the client and server certificate
openssl x509 -req \
             -sha256 \
             -days 3650 \
             -in example-server.csr \
             -signkey example-server-key.pem \
             -out example-server-cert.pem \
             -extensions req_ext \
             -CA example-CA-cert.pem \
             -CAkey example-CA-key.pem \
             -CAcreateserial \
             -extfile ssl.conf
openssl x509 -req \
             -sha256 \
             -days 3650 \
             -in example-client.csr \
             -signkey example-client-key.pem \
             -out example-client-cert.pem \
             -extensions req_ext \
             -CA example-CA-cert.pem \
             -CAkey example-CA-key.pem \
             -CAcreateserial \
             -extfile ssl.conf

# cleanup
rm example-CA-key.pem
rm example-CA-cert.srl
rm example-client.csr
rm example-server.csr
rm ssl.conf
rm wrong-ssl.conf
```

The server name (under alt_names in the ssl.conf) is `example.com`. (in accordance to [RFC 2006](https://tools.ietf.org/html/rfc2606))
