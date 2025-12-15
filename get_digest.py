import urllib.request
import json
import ssl

def get_digest(repo, tag):
    # 1. Get Token
    auth_url = f"https://auth.docker.io/token?service=registry.docker.io&scope=repository:{repo}:pull"
    try:
        with urllib.request.urlopen(auth_url) as r:
            token = json.loads(r.read().decode())['token']
    except Exception as e:
        print(f"Error getting token: {e}")
        return

    # 2. Get Manifest
    manifest_url = f"https://registry-1.docker.io/v2/{repo}/manifests/{tag}"
    req = urllib.request.Request(manifest_url)
    req.add_header("Authorization", f"Bearer {token}")
    # Accept header for multi-arch manifest or v2 manifest
    req.add_header("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
    # Also accept normal v2
    req.add_header("Accept", "application/vnd.docker.distribution.manifest.v2+json")
    
    try:
        with urllib.request.urlopen(req) as r:
            digest = r.headers.get('Docker-Content-Digest')
            print(f"Digest for {repo}:{tag} is {digest}")
    except Exception as e:
        print(f"Error getting manifest: {e}")

get_digest("library/alpine", "3.23.0")
