use crate::prelude::*;
use crate::network::gateway_types::{VmGatewayEndpoint, GatewayProtocol, GatewayForwardRequest};

pub(crate) fn forward_http_to_vm(
    endpoint: &VmGatewayEndpoint,
    req: &GatewayForwardRequest,
) -> Result<JsonValue, String> {
    let port = endpoint
        .protocol_port(GatewayProtocol::Http)
        .ok_or_else(|| "missing VM HTTP port in gateway endpoint".to_string())?;
    let url = format!(
        "http://{}:{}{}",
        endpoint.host,
        port,
        normalize_gateway_path(&req.path)
    );

    let method = Method::from_bytes(req.method.as_bytes())
        .map_err(|e| format!("invalid HTTP method '{}': {}", req.method, e))?;
    let client = Client::new();
    let mut builder = client.request(method, &url).body(req.body.clone());
    for (k, v) in &req.headers {
        builder = builder.header(k, v);
    }

    let response = builder
        .send()
        .map_err(|e| format!("gateway HTTP forwarding failed: {}", e))?;
    let status = response.status().as_u16();
    let body = response
        .bytes()
        .map_err(|e| format!("failed to read forwarded HTTP response body: {}", e))?;

    Ok(json!({
        "ok": true,
        "protocol": "http",
        "runtime": format!("{:?}", endpoint.runtime).to_lowercase(),
        "machineId": endpoint.machine_id,
        "vmId": endpoint.vm_id,
        "status": status,
        "bodyBase64": BASE64_STANDARD.encode(body),
    }))
}

pub(crate) fn normalize_gateway_path(path: &str) -> String {
    if path.trim().is_empty() {
        return "/".to_string();
    }
    if path.starts_with('/') {
        return path.to_string();
    }
    format!("/{}", path)
}
