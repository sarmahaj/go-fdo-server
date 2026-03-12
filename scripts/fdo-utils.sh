#! /bin/bash

get_rendezvous_info() {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
    --request GET \
    --header 'Content-Type: text/plain' \
    "${manufacturer_url}/api/v1/rvinfo"
}

set_rendezvous_info() {
  local manufacturer_url=$1
  local rv_info_v1=$2
  curl --fail --verbose --silent --insecure \
    --request POST \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_v1}" \
    "${manufacturer_url}/api/v1/rvinfo"
}

update_rendezvous_info() {
  local manufacturer_url=$1
  local rv_info_v1=$2
  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_v1}" \
    "${manufacturer_url}/api/v1/rvinfo"
}

delete_rendezvous_info() {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
    --request DELETE \
    "${manufacturer_url}/api/v1/rvinfo"
}

# === V2 API Functions (OpenAPI format) ===

get_rendezvous_info_v2() {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
    --request GET \
    "${manufacturer_url}/api/v2/rvinfo"
}

set_rendezvous_info_v2() {
  local manufacturer_url=$1
  local rv_info_v2=$2
  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_v2}" \
    "${manufacturer_url}/api/v2/rvinfo"
}

update_rendezvous_info_v2() {
  local manufacturer_url=$1
  local rv_info_v2=$2
  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_v2}" \
    "${manufacturer_url}/api/v2/rvinfo"
}

delete_rendezvous_info_v2() {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
    --request DELETE \
    "${manufacturer_url}/api/v2/rvinfo"
}

set_rendezvous_info_bypass_v2() {
  local manufacturer_url=$1
  local dns=$2
  local ip=$3
  local port=$4
  local protocol=$5

  # V2 format: array of arrays with rv_bypass flag, integer ports
  rv_info_bypass="[[{\"dns\":\"${dns}\"},{\"ip\":\"${ip}\"},{\"device_port\":${port}},{\"owner_port\":${port}},{\"protocol\":\"${protocol}\"},{\"rv_bypass\":true}]]"

  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_bypass}" \
    "${manufacturer_url}/api/v2/rvinfo"
}

get_owner_redirect_info() {
  local owner_url=$1
  curl --fail --verbose --silent --insecure \
    --header 'Content-Type: text/plain' \
    "${owner_url}/api/v1/owner/redirect"
}

set_owner_redirect_info() {
  local owner_url=$1
  local ip=$2
  local dns=$3
  local port=$4
  # TransportProtocol /= (
  #     ProtTCP:    1,     ;; bare TCP stream
  #     ProtTLS:    2,     ;; bare TLS stream
  #     ProtHTTP:   3,
  #     ProtCoAP:   4,
  #     ProtHTTPS:  5,
  #     ProtCoAPS:  6,
  # )
  local protocol=$5
  rvto2addr="[{\"ip\": \"${ip}\", \"dns\": \"${dns}\", \"port\": \"${port}\", \"protocol\": \"${protocol}\"}]"
  curl --fail --verbose --silent --insecure \
    --request POST \
    --header 'Content-Type: text/plain' \
    --data-raw "${rvto2addr}" \
    "${owner_url}/api/v1/owner/redirect"
}

update_owner_redirect_info() {
  local owner_url=$1
  local ip=$2
  local dns=$3
  local port=$4
  # TransportProtocol /= (
  #     ProtTCP:    1,     ;; bare TCP stream
  #     ProtTLS:    2,     ;; bare TLS stream
  #     ProtHTTP:   3,
  #     ProtCoAP:   4,
  #     ProtHTTPS:  5,
  #     ProtCoAPS:  6,
  # )
  local protocol=$5
  rvto2addr="[{\"ip\": \"${ip}\", \"dns\": \"${dns}\", \"port\": \"${port}\", \"protocol\": \"${protocol}\"}]"
  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: text/plain' \
    --data-raw "${rvto2addr}" \
    "${owner_url}/api/v1/owner/redirect"
}

get_ov_from_manufacturer() {
  local manufacturer_url=$1
  local guid=$2
  local output=$3
  curl --fail --verbose --silent --insecure \
    "${manufacturer_url}/api/v1/vouchers/${guid}" -o "${output}"
}

send_ov_to_owner() {
  local owner_url=$1
  local output=$2
  [ -s "${output}" ] || {
    echo "❌ Voucher file not found or empty: ${output}" >&2
    return 1
  }
  curl --fail --verbose --silent --insecure \
    --request POST \
    --data-binary "@${output}" \
    "${owner_url}/api/v1/owner/vouchers"
}

resell() {
  local owner_url=$1
  local guid=$2
  local new_owner_pubkey=$3
  local output=$4
  [ -s "${new_owner_pubkey}" ] || {
    echo "❌ Public key file not found or empty: ${new_owner_pubkey}" >&2
    return 1
  }
  curl --fail --verbose --silent --insecure "${owner_url}/api/v1/owner/resell/${guid}" --data-binary @"${new_owner_pubkey}" -o "${output}"
}

get_device_ca_certs() {
  local url=$1
  curl --fail --verbose --silent --insecure \
    --request GET \
    "${url}/api/v1/device-ca"
}

add_device_ca_cert() {
  local url=$1
  local crt=$2
  curl --fail --verbose --silent --insecure \
    --request POST \
    --header 'Content-Type: application/x-pem-file' \
    --data-binary @"${crt}" \
    "${url}/api/v1/device-ca"
}

delete_device_ca_cert() {
  local url=$1
  local fingerprint=$2
  curl --fail --verbose --silent --insecure \
    --request DELETE --header 'Content-Type: application/x-pem-file' \
    "${url}/api/v1/device-ca/${fingerprint}"
}
