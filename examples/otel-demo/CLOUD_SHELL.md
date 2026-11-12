<!--
Copyright (c) 2026 The Jaeger Authors.
SPDX-License-Identifier: Apache-2.0
-->

# Bootstrap AI credentials from Oracle Cloud Shell

Use this procedure when the demo's OKE context is available in Oracle Cloud
Shell but not on the operator's local machine. It installs only
`jaeger/jaeger-demo-ai`; the normal deployment workflow handles workload upgrades.
The workflow preserves the Secret, so provider credentials do not need a second
copy in GitHub Actions secrets.

Complete the project, model, billing, and spending-control checks in
[AI_ASSISTANT.md](AI_ASSISTANT.md) first. Prepare a private local env file with
exactly `OPENAI_API_KEY` and `GOOSE_SERVER__SECRET_KEY`, using mode `0600`.
The provider key must belong to the demo project's service account. The agent
secret must be independently generated, with at least 32 random bytes.

## Verify the target in Cloud Shell

Sign in to the intended tenancy, open Cloud Shell, and run these commands
individually. Replace `YOUR_OKE_CONTEXT` with the context shown by kubectl:

```bash
kubectl config get-contexts
demo_context=YOUR_OKE_CONTEXT
kubectl --context "$demo_context" -n jaeger get ingress \
  -o 'custom-columns=NAME:.metadata.name,HOSTS:.spec.rules[*].host'
kubectl --context "$demo_context" -n jaeger get secret jaeger-demo-ai \
  --ignore-not-found -o name
```

Confirm the ingress contains `jaeger.demo.jaegertracing.io`. If the Secret already
exists, use the runbook's rotation procedure rather than overwriting it as an
initial bootstrap. Do not print Secret data or dump the kubeconfig.

## Transfer an encrypted credential file

Cloud Shell's upload dialog writes into the home directory. Encrypt the file
locally, then decrypt it in a private temporary directory. This keeps plaintext
credentials out of the upload area, terminal history, and browser output.

In Cloud Shell, create the private directory and a temporary RSA transfer key:

```bash
umask 077
demo_secret_dir=$(mktemp -d "$HOME/jaeger-demo-ai.XXXXXX")
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 \
  -out "$demo_secret_dir/transport-key.pem"
openssl pkey -in "$demo_secret_dir/transport-key.pem" -pubout
printf 'Private directory: %s\n' "$demo_secret_dir"
```

Copy only the displayed **public** key into a local PEM file. Keep the private
key in Cloud Shell. Record the temporary directory path for the remaining steps.

On the local machine, use Node.js 18 or newer to encrypt the env file. Replace
both absolute input paths below. The command prints only the encrypted file's
path; it never prints the credentials:

```bash
node - /absolute/path/private-demo.env /absolute/path/transport-public.pem <<'NODE'
const fs = require('node:fs');
const crypto = require('node:crypto');
const os = require('node:os');
const path = require('node:path');
const [sourcePath, publicKeyPath] = process.argv.slice(2);
const key = crypto.createPublicKey(fs.readFileSync(publicKeyPath));
if (key.asymmetricKeyType !== 'rsa' || key.asymmetricKeyDetails.modulusLength !== 4096) {
  throw new Error('Expected the temporary RSA-4096 public key');
}
const source = fs.readFileSync(sourcePath);
if (source.length > 446) throw new Error('Env file exceeds the RSA-OAEP transfer size');
const encrypted = crypto.publicEncrypt({
  key, oaepHash: 'sha256', padding: crypto.constants.RSA_PKCS1_OAEP_PADDING,
}, source);
const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'jaeger-ai-transfer-'));
const output = path.join(directory, 'jaeger-demo-ai-' + crypto.randomBytes(8).toString('hex') + '.bin');
fs.writeFileSync(output, encrypted, {mode: 0o600, flag: 'wx'});
console.log(output);
NODE
```

In Cloud Shell's **Menu → Upload**, select the encrypted `.bin` file. After the
upload completes, set its path in the Cloud Shell terminal, using the actual
uploaded filename:

```bash
demo_cipher_file="$HOME/jaeger-demo-ai-UPLOADED_NAME.bin"
openssl pkeyutl -decrypt -inkey "$demo_secret_dir/transport-key.pem" \
  -in "$demo_cipher_file" -out "$demo_secret_dir/credentials.env" \
  -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256
chmod 600 "$demo_secret_dir/credentials.env"
```

Only the temporary private key can decrypt the upload. The 446-byte limit keeps
this transfer bounded to the two-credential env file; do not add other secrets.
Stop if encryption or decryption fails.

## Install and verify the Secret

Create the Secret using the private file. This command prints only the resource
name and does not put credential values in command arguments:

```bash
kubectl --context "$demo_context" -n jaeger create secret generic jaeger-demo-ai \
  --from-env-file="$demo_secret_dir/credentials.env"
```

Verify both stored values against the source in memory, without displaying them:

```bash
python3 - "$demo_context" "$demo_secret_dir/credentials.env" <<'PY'
import base64
import json
from pathlib import Path
import subprocess
import sys

context, source = sys.argv[1:]
expected = dict(line.split("=", 1) for line in Path(source).read_text().splitlines() if "=" in line)
stored = json.loads(subprocess.check_output([
    "kubectl", "--context", context, "-n", "jaeger", "get", "secret",
    "jaeger-demo-ai", "-o", "json",
], universal_newlines=True))
actual = {key: base64.b64decode(value, validate=True).decode() for key, value in stored["data"].items()}
assert set(actual) == {"OPENAI_API_KEY", "GOOSE_SERVER__SECRET_KEY"}, "Unexpected Secret entries"
assert actual == expected, "Stored credentials differ from the private source"
print("Verified jaeger/jaeger-demo-ai: both credentials match the private source")
PY
```

After successful verification, remove the temporary cloud files:

```bash
rm -- "$demo_cipher_file" "$demo_secret_dir/credentials.env" "$demo_secret_dir/transport-key.pem"
rmdir -- "$demo_secret_dir"
```

Remove the local encrypted transfer file and its temporary directory too. Keep
the original private credential file in the operator's approved secret storage.

The Secret is now ready for the normal Jaeger-only upgrade. Its creation does
not start Goose or enable public AI traffic. Finish image publication, account
usage-limit checks, and the rollout verification in
[AI_ASSISTANT.md](AI_ASSISTANT.md) before enabling the assistant.
