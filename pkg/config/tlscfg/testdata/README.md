# Example Certificate Authority and Certificate creation for testing

The following commands were used to create the CA, server and client's certificates and keys

```bash

# create CA
openssl genrsa -out example-CA-key.pem  2048
openssl req -new -key example-CA-key.pem -x509 -days 3650 -out example-CA-cert.pem -subj /CN="example-CA"

# create Wrong CA (a dummy CA which doesn't provide any certificate )
openssl genrsa -out wrong-CA-key.pem  2048
openssl req -new -key wrong-CA-key.pem -x509 -days 3650 -out wrong-CA-cert.pem -subj /CN="wrong-CA"

# cerating client and server keys
openssl genrsa -out example-server-key.pem 2048
openssl genrsa -out example-client-key.pem 2048

# creating certificate sign request  using the above created keys and configuration given and commandline arguemnts
openssl req -new -nodes -key example-server-key.pem -out example-server.csr -subj /CN="example.com"  # This server's name is provided as parameter during client's tls configuration  
openssl req -new -nodes -key example-client-key.pem -out example-client.csr -subj /CN="example-client"

# creating the client and server certificate
openssl x509 -req -in example-server.csr -CA example-CA-cert.pem -CAkey example-CA-key.pem -CAcreateserial -out example-server-cert.pem
openssl x509 -req -in example-client.csr -CA example-CA-cert.pem -CAkey example-CA-key.pem -CAcreateserial -out example-client-cert.pem

# cleanup
rm example-CA-cert.srl
rm example-client.csr
rm example-server.csr
```

The server name (common name in the server certificate ) is `example.com` . (in accordance to [RFC 2006](https://tools.ietf.org/html/rfc2606) )  
The common name of the client is `example-client` (never actually used).  
The common name of the Certificate Authority (CA) is `example-CA`  .  
