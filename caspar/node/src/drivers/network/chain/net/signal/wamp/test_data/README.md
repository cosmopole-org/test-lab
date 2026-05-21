# WAMP Test Data Certificates 🔐

Use the command below to generate a self-signed certificate for local test data.

```bash
# key.pem: unencrypted private key used to sign certificate
# cert.pem: generated certificate
openssl req -x509 \
  -subj "/C=GB/CN=foo.co.uk" \
  -addext "subjectAltName = DNS:foo.co.uk" \
  -newkey rsa:4096 \
  -nodes \
  -keyout key.pem \
  -out cert.pem \
  -days 3650
```
