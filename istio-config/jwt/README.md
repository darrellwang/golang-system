# Dev JWT (RS256)

**仅用于本地 / 联调。** 私钥已检入仅为演示；生产必须换成真实 IdP 的 `jwksUri`，并删除 inline JWKS。

| 字段 | 值 |
|---|---|
| issuer | `golang-system-dev` |
| audience | `golang-system` |
| alg | RS256 |
| kid | `rOxTWJqbhHs` |

## 签发测试 Token

```bash
# 需要 Python 3 + cryptography
pip install cryptography

python3 istio-config/jwt/mint-dev-jwt.py \
  --sub user-1 \
  --org org-1 \
  --email user@example.com
```

输出一行 JWT，调用网关时带上：

```bash
TOKEN=$(python3 istio-config/jwt/mint-dev-jwt.py --sub user-1 --org org-1)
curl -H "Authorization: Bearer $TOKEN" -H "Host: api.example.com" \
  https://$INGRESS_IP/rest/v1/items
```

无效或缺失 JWT 访问 `/rest/*` 会得到 401/403（由 Ingress Gateway 的 AuthorizationPolicy 拦截）。
