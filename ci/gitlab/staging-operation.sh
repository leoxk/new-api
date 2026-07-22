#!/usr/bin/env bash
set -Eeuo pipefail

: "${STAGING_OPERATION:?set STAGING_OPERATION for this manual job}"
: "${STAGING_PUBLIC_HEALTH_URL:?missing staging public health URL}"
base_url=${STAGING_PUBLIC_HEALTH_URL%/api/status}

admin_headers=()
if [ "$STAGING_OPERATION" != verify_b2b_catalog_gate ]; then
  : "${STAGING_ADMIN_ACCESS_TOKEN:?missing staging admin access token}"
  admin_headers=(
    -H "Authorization: Bearer $STAGING_ADMIN_ACCESS_TOKEN"
    -H "New-Api-User: 1"
  )
fi

case "$STAGING_OPERATION" in
  sync_test_customer)
    : "${STAGING_CUSTOMER_PASSWORD:?missing controlled staging customer password}"
    user_json=$(curl --fail-with-body --silent --show-error \
      "${admin_headers[@]}" "$base_url/api/user/3")
    jq -e '.success == true and .data.id == 3 and .data.username == "b2btest" and .data.group == "b2b"' \
      <<<"$user_json" >/dev/null
    payload=$(jq -cn --arg password "$STAGING_CUSTOMER_PASSWORD" '{
      id: 3,
      username: "b2btest",
      display_name: "B2B Test Customer",
      password: $password,
      group: "b2b",
      remark: "Controlled B2B payment staging customer"
    }')
    update_json=$(curl --fail-with-body --silent --show-error \
      -X PUT "${admin_headers[@]}" -H 'Content-Type: application/json' \
      --data "$payload" "$base_url/api/user/")
    jq -e '.success == true' <<<"$update_json" >/dev/null
    login_json=$(curl --fail-with-body --silent --show-error \
      -X POST -H 'Content-Type: application/json' \
      --data "$(jq -cn --arg password "$STAGING_CUSTOMER_PASSWORD" \
        '{username:"b2btest", password:$password}')" \
      "$base_url/api/user/login")
    jq -e '.success == true and .data.id == 3 and .data.group == "b2b"' \
      <<<"$login_json" >/dev/null
    echo "Controlled staging customer reset and login verification passed"
    ;;

  record_completed_stripe_refund)
    : "${STAGING_TRADE_NO:?missing staging top-up order number}"
    : "${STAGING_PROVIDER_REFUND_ID:?missing completed Stripe refund ID}"
    : "${STAGING_REFUNDED_MONEY:?missing refund amount}"
    : "${STAGING_REFUND_REASON:?missing refund reason}"
    [[ "$STAGING_TRADE_NO" =~ ^ref_[A-Za-z0-9]+$ ]]
    [[ "$STAGING_PROVIDER_REFUND_ID" =~ ^re_[A-Za-z0-9]+$ ]]
    [[ "$STAGING_REFUNDED_MONEY" =~ ^[0-9]+([.][0-9]{1,2})?$ ]]
    awk -v amount="$STAGING_REFUNDED_MONEY" 'BEGIN { exit !(amount > 0) }'
    order_json=$(curl --fail-with-body --silent --show-error \
      "${admin_headers[@]}" --get \
      --data-urlencode "keyword=$STAGING_TRADE_NO" \
      --data-urlencode page=1 --data-urlencode page_size=20 \
      "$base_url/api/user/topup")
    jq -e --arg trade_no "$STAGING_TRADE_NO" --arg amount "$STAGING_REFUNDED_MONEY" '
      .success == true and
      ([.data.items[] | select(.trade_no == $trade_no)] | length) == 1 and
      ([.data.items[] | select(.trade_no == $trade_no)][0] |
        .user_id == 3 and .status == "success" and
        (.payment_provider == "stripe" or .payment_method == "stripe") and
        (.refunded_money | tonumber) == 0 and .provider_refund_id == "" and
        (.money | tonumber) >= ($amount | tonumber))
    ' <<<"$order_json" >/dev/null
    payload=$(jq -cn \
      --arg trade_no "$STAGING_TRADE_NO" \
      --arg refunded_money "$STAGING_REFUNDED_MONEY" \
      --arg provider_refund_id "$STAGING_PROVIDER_REFUND_ID" \
      --arg reason "$STAGING_REFUND_REASON" '{
        trade_no:$trade_no,
        refunded_money:$refunded_money,
        provider_refund_id:$provider_refund_id,
        reason:$reason
      }')
    refund_json=$(curl --fail-with-body --silent --show-error \
      -X POST "${admin_headers[@]}" -H 'Content-Type: application/json' \
      --data "$payload" "$base_url/api/user/topup/refund-record")
    jq -e --arg trade_no "$STAGING_TRADE_NO" \
      --arg provider_refund_id "$STAGING_PROVIDER_REFUND_ID" \
      --arg amount "$STAGING_REFUNDED_MONEY" '
        .success == true and .data.trade_no == $trade_no and
        .data.provider_refund_id == $provider_refund_id and
        (.data.refunded_money | tonumber) == ($amount | tonumber)
      ' <<<"$refund_json" >/dev/null
    echo "Completed Stripe Sandbox refund recorded and verified"
    ;;

  sync_b2b_catalog_policy)
    options=$(curl --fail-with-body --silent --show-error \
      "${admin_headers[@]}" "$base_url/api/option/")
    jq -e '.success == true' <<<"$options" >/dev/null
    auto=$(jq -cer '[.data[] | select(.key == "AutoGroups")][0].value | fromjson |
      . + ["b2b", "b2b-deepseek"] | unique' <<<"$options")
    exposed=$(jq -cer '[.data[] | select(.key == "UserUsableGroups")][0].value |
      fromjson | del(.b2b, ."b2b-deepseek")' <<<"$options")
    special=$(jq -cer '[.data[] |
      select(.key == "group_ratio_setting.group_special_usable_group")][0].value |
      fromjson | .b2b = {
        "-:default":"", "+:auto":"Auto", "+:b2b-deepseek":"DeepSeek"
      }' <<<"$options")
    while IFS= read -r option; do
      result=$(curl --fail-with-body --silent --show-error \
        -X PUT "${admin_headers[@]}" -H 'Content-Type: application/json' \
        --data "$option" "$base_url/api/option/")
      jq -e '.success == true' <<<"$result" >/dev/null
    done < <(printf '%s\n' \
      "$(jq -cn --arg value "$auto" '{key:"AutoGroups",value:$value}')" \
      "$(jq -cn --arg value "$exposed" '{key:"UserUsableGroups",value:$value}')" \
      "$(jq -cn --arg value "$special" \
        '{key:"group_ratio_setting.group_special_usable_group",value:$value}')")
    echo "Staging B2B catalog policy synchronized"
    ;;

  verify_b2b_catalog_gate)
    : "${STAGING_B2B_TEST_API_KEY:?missing controlled B2B test API key}"
    models=$(curl --fail-with-body --silent --show-error \
      -H "Authorization: Bearer $STAGING_B2B_TEST_API_KEY" \
      "$base_url/v1/models")
    if jq -e '.data[]? | select(.id == "gpt-image-1" or .id == "gpt-image-2")' \
      <<<"$models" >/dev/null; then
      echo "deferred GPT Image model remains visible in staging" >&2
      exit 1
    fi
    response=$(mktemp)
    trap 'rm -f "$response"' EXIT
    status=$(curl --silent --show-error --output "$response" --write-out '%{http_code}' \
      -X POST -H "Authorization: Bearer $STAGING_B2B_TEST_API_KEY" \
      -H 'Content-Type: application/json' \
      --data '{"model":"gpt-image-2","prompt":"staging catalog gate check"}' \
      "$base_url/v1/images/generations")
    [ "$status" = 404 ]
    jq -e '.error.code == "model_not_found" and
      (.error.message | contains("not available for this customer group"))' \
      "$response" >/dev/null
    echo "Staging B2B catalog gate verified"
    ;;

  *)
    echo "unsupported STAGING_OPERATION: $STAGING_OPERATION" >&2
    echo "allowed: sync_test_customer, record_completed_stripe_refund, sync_b2b_catalog_policy, verify_b2b_catalog_gate" >&2
    exit 2
    ;;
esac
