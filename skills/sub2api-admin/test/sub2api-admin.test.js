const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const http = require("node:http");
const path = require("node:path");
const test = require("node:test");

const cliPath = path.join(__dirname, "..", "scripts", "sub2api-admin.js");

function jsonResponse(res, data) {
  res.writeHead(200, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ code: 0, data }));
}

function runCli(baseURL, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [cliPath, ...args], {
      env: {
        ...process.env,
        SUB2API_BASE_URL: baseURL,
        SUB2API_ADMIN_API_KEY: "test-admin-key",
        SUB2API_JWT: "",
      },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.on("error", reject);
    child.on("close", (code) => resolve({ code, stdout, stderr }));
  });
}

async function withServer(handler, callback) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  try {
    return await callback(`http://127.0.0.1:${address.port}`);
  } finally {
    await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  }
}

test("proxies all emits only allowlisted fields and credential presence", async () => {
  await withServer((req, res) => {
    assert.equal(req.method, "GET");
    assert.equal(req.url, "/api/v1/admin/proxies/all");
    jsonResponse(res, [{
      id: 4,
      name: "egress-04",
      protocol: "socks5h",
      host: "10.20.0.11",
      port: 1080,
      username: "synthetic-user",
      password: "synthetic-password",
      status: "active",
      account_count: 0,
      latency_message: "must-not-leak-latency",
      quality_summary: "must-not-leak-quality",
      unexpected: "must-not-pass",
    }]);
  }, async (baseURL) => {
    const result = await runCli(baseURL, ["proxies", "all"]);
    assert.equal(result.code, 0, result.stderr);
    assert.equal(result.stderr, "");
    const output = JSON.parse(result.stdout);
    assert.deepEqual(output, [{
      id: 4,
      name: "egress-04",
      protocol: "socks5h",
      host: "10.20.0.11",
      port: 1080,
      status: "active",
      account_count: 0,
      auth_present: true,
      password_present: true,
    }]);
    assert.equal(result.stdout.includes("synthetic-user"), false);
    assert.equal(result.stdout.includes("synthetic-password"), false);
    assert.equal(result.stdout.includes("unexpected"), false);
    assert.equal(result.stdout.includes("must-not-leak-latency"), false);
    assert.equal(result.stdout.includes("must-not-leak-quality"), false);
    assert.equal(Object.prototype.hasOwnProperty.call(output[0], "username"), false);
    assert.equal(Object.prototype.hasOwnProperty.call(output[0], "password"), false);
  });
});

test("proxies create sends the exact idempotency header and redacts its response", async () => {
  const idempotencyKey = "proxy-egress-04-20260722";
  await withServer(async (req, res) => {
    assert.equal(req.method, "POST");
    assert.equal(req.url, "/api/v1/admin/proxies");
    assert.equal(req.headers["idempotency-key"], idempotencyKey);
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    assert.equal(body.name, "egress-04");
    jsonResponse(res, {
      id: 4,
      ...body,
      status: "active",
      username: "synthetic-user",
      password: "synthetic-password",
    });
  }, async (baseURL) => {
    const payload = JSON.stringify({
      name: "egress-04",
      protocol: "socks5h",
      host: "10.20.0.11",
      port: 1080,
      fallback_mode: "none",
    });
    const result = await runCli(baseURL, [
      "proxies",
      "create",
      "--json",
      payload,
      "--idempotency-key",
      idempotencyKey,
    ]);
    assert.equal(result.code, 0, result.stderr);
    const output = JSON.parse(result.stdout);
    assert.equal(output.id, 4);
    assert.equal(output.auth_present, true);
    assert.equal(output.password_present, true);
    assert.equal(Object.prototype.hasOwnProperty.call(output, "username"), false);
    assert.equal(Object.prototype.hasOwnProperty.call(output, "password"), false);
  });
});

test("proxies create fails closed before a request when idempotency key is absent", async () => {
  let requests = 0;
  await withServer((req, res) => {
    requests += 1;
    jsonResponse(res, {});
  }, async (baseURL) => {
    const result = await runCli(baseURL, [
      "proxies",
      "create",
      "--json",
      JSON.stringify({ name: "egress-04", protocol: "socks5h", host: "10.20.0.11", port: 1080 }),
    ]);
    assert.equal(result.code, 1);
    assert.match(result.stderr, /requires a non-empty --idempotency-key/);
    assert.equal(result.stdout, "");
    assert.equal(requests, 0);
  });
});

test("raw api sends an optional idempotency header unchanged", async () => {
  const idempotencyKey = "ops-approved-proxy-create-04";
  await withServer((req, res) => {
    assert.equal(req.method, "POST");
    assert.equal(req.url, "/api/v1/admin/example");
    assert.equal(req.headers["idempotency-key"], idempotencyKey);
    jsonResponse(res, { accepted: true });
  }, async (baseURL) => {
    const result = await runCli(baseURL, [
      "api",
      "POST",
      "/admin/example",
      "--json",
      "{}",
      "--idempotency-key",
      idempotencyKey,
    ]);
    assert.equal(result.code, 0, result.stderr);
    assert.deepEqual(JSON.parse(result.stdout), { accepted: true });
  });
});

test("proxy lookup, test, accounts, and delete use their documented routes", async () => {
  const routes = [];
  const outputs = [];
  await withServer((req, res) => {
    routes.push(`${req.method} ${req.url}`);
    if (req.url.endsWith("/test")) {
      jsonResponse(res, { success: true, message: "must-not-leak-test", latency_ms: 8, ip_address: "192.0.2.4" });
    } else if (req.url.endsWith("/accounts")) {
      jsonResponse(res, [{ id: 9, name: "account-9", platform: "openai", type: "oauth", notes: "must-not-leak-note" }]);
    } else if (req.method === "DELETE") {
      jsonResponse(res, { message: "Proxy deleted successfully" });
    } else {
      jsonResponse(res, {
        id: 4,
        name: "egress-04",
        protocol: "socks5h",
        host: "10.20.0.11",
        port: 1080,
        username: "synthetic-user",
        password: "synthetic-password",
      });
    }
  }, async (baseURL) => {
    for (const args of [
      ["proxies", "get", "4"],
      ["proxies", "test", "4"],
      ["proxies", "accounts", "4"],
      ["proxies", "delete", "4"],
    ]) {
      const result = await runCli(baseURL, args);
      assert.equal(result.code, 0, result.stderr);
      outputs.push(JSON.parse(result.stdout));
    }
  });
  assert.deepEqual(routes, [
    "GET /api/v1/admin/proxies/4",
    "POST /api/v1/admin/proxies/4/test",
    "GET /api/v1/admin/proxies/4/accounts",
    "DELETE /api/v1/admin/proxies/4",
  ]);
  const serialized = JSON.stringify(outputs);
  assert.equal(serialized.includes("must-not-leak-test"), false);
  assert.equal(serialized.includes("must-not-leak-note"), false);
});
