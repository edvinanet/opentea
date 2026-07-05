# Manual smoke test

This walks through the same 11-step worked example the automated integration
test (`cmd/opentea/integration_test.go`) drives, as copy-pasteable `curl`
commands, so you can see it work against a real running server.

## 1. Bootstrap an admin and start the server

```bash
go run ./cmd/opentea createadmin -username=admin -password=adminpass123
go run ./cmd/opentea
```

By default it listens on `:8080`, stores metadata in `data/opentea.db`
(SQLite), and stores uploaded files under `data/blobs/`. Override with the
`TEA_LISTEN_ADDR`, `TEA_DB_PATH`, `TEA_BLOB_DIR`, `TEA_ROOT_URL`, and
`TEA_VERSIONS` environment variables.

Set `BASE=http://localhost:8080` for the commands below.

`/admin/v1` now requires authentication (see [README-admin.md](README-admin.md)) — log in once
and reuse the cookie jar for every `/admin/v1` call below:

```bash
curl -s -c cookies.txt -X POST $BASE/admin/ui/login -d 'username=admin&password=adminpass123'
```

## 2. Create a product

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/products -H 'Content-Type: application/json' -d '{
  "name": "Acme Widget",
  "identifiers": [{"idType": "PURL", "idValue": "pkg:generic/acme-widget"}]
}' | tee /tmp/product.json
PRODUCT_UUID=$(jq -r .uuid /tmp/product.json)
```

## 3. Create a product release

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/products/$PRODUCT_UUID/releases -H 'Content-Type: application/json' -d '{
  "version": "1.0.0",
  "createdDate": "2026-07-01T00:00:00Z",
  "releaseDate": "2026-07-01T00:00:00Z",
  "identifiers": [{"idType": "TEI", "idValue": "urn:tei:uuid:acme.example.com:widget-1.0.0"}]
}' | tee /tmp/product_release.json
PRODUCT_RELEASE_UUID=$(jq -r .uuid /tmp/product_release.json)
```

## 4. Create a component

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/components -H 'Content-Type: application/json' -d '{
  "name": "acme-widget-core"
}' | tee /tmp/component.json
COMPONENT_UUID=$(jq -r .uuid /tmp/component.json)
```

## 5. Create a component release

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/components/$COMPONENT_UUID/releases -H 'Content-Type: application/json' -d '{
  "version": "1.0.0",
  "createdDate": "2026-07-01T00:00:00Z"
}' | tee /tmp/component_release.json
COMPONENT_RELEASE_UUID=$(jq -r .uuid /tmp/component_release.json)
```

## 6. Add a distribution and upload its file

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/componentReleases/$COMPONENT_RELEASE_UUID/distributions -H 'Content-Type: application/json' -d '{
  "description": "linux/amd64 tarball"
}' | tee /tmp/distribution.json
DISTRIBUTION_ID=$(jq -r .distributionId /tmp/distribution.json)

echo "fake tarball bytes" > /tmp/widget.tar.gz
curl -s -b cookies.txt -X POST $BASE/admin/v1/distributions/$DISTRIBUTION_ID/files -F "file=@/tmp/widget.tar.gz;type=application/gzip"
```

## 7. Link the component to the product release

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/productReleases/$PRODUCT_RELEASE_UUID/components -H 'Content-Type: application/json' -d "{
  \"uuid\": \"$COMPONENT_UUID\",
  \"release\": \"$COMPONENT_RELEASE_UUID\"
}"
```

## 8. Create an artifact and upload its file

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/artifacts -H 'Content-Type: application/json' -d "{
  \"name\": \"cyclonedx-sbom.json\",
  \"type\": \"BOM\",
  \"createdDate\": \"2026-07-01T00:00:00Z\",
  \"distributionIds\": [\"$DISTRIBUTION_ID\"],
  \"formats\": [{\"mediaType\": \"application/vnd.cyclonedx+json\"}]
}" | tee /tmp/artifact.json
ARTIFACT_UUID=$(jq -r .uuid /tmp/artifact.json)
ARTIFACT_VERSION=$(jq -r .version /tmp/artifact.json)

echo '{"bomFormat":"CycloneDX"}' > /tmp/sbom.json
curl -s -b cookies.txt -X POST "$BASE/admin/v1/artifacts/$ARTIFACT_UUID/$ARTIFACT_VERSION/files?formatIndex=0" -F "file=@/tmp/sbom.json;type=application/vnd.cyclonedx+json"
```

## 9. Create a collection referencing the artifact

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/componentReleases/$COMPONENT_RELEASE_UUID/collections -H 'Content-Type: application/json' -d "{
  \"updateReason\": {\"type\": \"INITIAL_RELEASE\", \"comment\": \"first collection\"},
  \"artifacts\": [{\"uuid\": \"$ARTIFACT_UUID\", \"version\": $ARTIFACT_VERSION}]
}"
```

## 10. Add a CLE "released" event to the product release

```bash
curl -s -b cookies.txt -X POST $BASE/admin/v1/productReleases/$PRODUCT_RELEASE_UUID/cle/events -H 'Content-Type: application/json' -d '{
  "type": "released",
  "effective": "2026-07-01T00:00:00Z",
  "published": "2026-07-01T00:00:00Z",
  "version": "1.0.0",
  "description": "Initial release"
}'
```

## 11. Read it all back through the spec-conformant API

```bash
curl -s $BASE/tea/v1/products | jq
curl -s $BASE/tea/v1/product/$PRODUCT_UUID/releases | jq
curl -s $BASE/tea/v1/productRelease/$PRODUCT_RELEASE_UUID | jq
curl -s $BASE/tea/v1/componentRelease/$COMPONENT_RELEASE_UUID | jq

# The artifact's file URL should resolve and byte-match what was uploaded:
ARTIFACT_URL=$(curl -s $BASE/tea/v1/componentRelease/$COMPONENT_RELEASE_UUID | jq -r .latestCollection.artifacts[0].formats[0].url)
curl -s $ARTIFACT_URL

curl -s $BASE/tea/v1/productRelease/$PRODUCT_RELEASE_UUID/cle | jq

# Discovery resolves the TEI to the product release:
curl -s "$BASE/tea/v1/discovery?tei=$(python3 -c "import urllib.parse; print(urllib.parse.quote('urn:tei:uuid:acme.example.com:widget-1.0.0'))")" | jq

# Error cases:
curl -s -o /dev/null -w '%{http_code}\n' $BASE/tea/v1/product/00000000-0000-4000-8000-000000000000  # 404
curl -s -o /dev/null -w '%{http_code}\n' $BASE/tea/v1/product/not-a-uuid                              # 400
```
