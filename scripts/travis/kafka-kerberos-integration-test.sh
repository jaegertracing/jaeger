#!/bin/bash

set -e

sudo apt-get update
sudo apt-get -y install krb5-user

docker pull rubensvp/kafka-kerberos

temp_dir=$(mktemp -d)

export KAFKA_KERBEROS_CONFIG=${temp_dir}/krb5.conf
export KAFKA_KERBEROS_COLLECTOR_KEYTAB=${temp_dir}/collector.keytab
export KAFKA_KERBEROS_INGESTER_KEYTAB=${temp_dir}/ingester.keytab

cat <<EOF > ${KAFKA_KERBEROS_CONFIG}
[libdefaults]
 dns_lookup_realm = false
 ticket_lifetime = 24h
 renew_lifetime = 7d
 forwardable = true
 rdns = false
 default_realm = EXAMPLE.COM

[realms]
EXAMPLE.COM = {
    kdc = 127.0.0.1
    admin_server = 127.0.0.1
 }
EOF

export KRB5_CONFIG=${KAFKA_KERBEROS_CONFIG}

CID=$(docker run -d -e REALM=EXAMPLE.COM -e KADMIN_PASSWORD=qwerty -p 88:88/udp -p 9092:9092 -p 464:464 -p 749:749 --rm rubensvp/kafka-kerberos)
# Guarantees no matter what happens, docker will remove the instance at the end.
trap 'docker rm -f $CID && rm -rf ${temp_dir} 2>/dev/null' EXIT INT TERM

# Create client keytab
REALM="EXAMPLE.COM"
KADMIN_PRINCIPAL="admin/admin"
KADMIN_PRINCIPAL_FULL=$KADMIN_PRINCIPAL@$REALM
KADMIN_PASSWORD="qwerty"

echo "REALM: $REALM"
echo "KADMIN_PRINCIPAL_FULL: $KADMIN_PRINCIPAL_FULL"
echo "KADMIN_PASSWORD: $KADMIN_PASSWORD"
echo ""

function kadminCommand {
    kadmin -p $KADMIN_PRINCIPAL_FULL -w $KADMIN_PASSWORD -q "$1"
}

echo "Add ingester user"
kadminCommand "addprinc -pw secret ingester/localhost@EXAMPLE.COM"
echo "Create ingester keytab"
kadminCommand "xst -k ${KAFKA_KERBEROS_INGESTER_KEYTAB} ingester/localhost"

echo "Add collector user"
kadminCommand "addprinc -pw secret collector/localhost@EXAMPLE.COM"
echo "Create collector keytab"
kadminCommand "xst -k ${KAFKA_KERBEROS_COLLECTOR_KEYTAB} collector/localhost"

sleep 30

make kafka-kerberos-integration-test