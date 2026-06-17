// k6 load test for Janus — measures the gateway's own overhead on the cache-hit
// path (auth → rate-limit → cache lookup → serve), which is the cheap, repeatable
// way to validate the latency bar: cache hits return in < 10 ms. It sends one
// fixed prompt, so after a single warm-up call every request is a cache hit and no
// provider tokens are spent.
//
// Run (needs a running gateway + a virtual key with a high/unlimited rate limit):
//   VK=sk-gw-xxxx GW=http://localhost:8080 k6 run loadtest/cache-hit.js
//   # tune load:  VUS=50 DURATION=1m  k6 run ...
//
// Tip: mint a dedicated load-test key with rate_limit_rpm = 0 (unlimited) so the
// limiter doesn't turn the run into 429s.

import http from "k6/http";
import { check } from "k6";
import { Trend } from "k6/metrics";

const GW = __ENV.GW || "http://localhost:8080";
const VK = __ENV.VK;

const latency = new Trend("gateway_latency_ms", true);

export const options = {
  scenarios: {
    cache_hits: {
      executor: "constant-vus",
      vus: Number(__ENV.VUS || 20),
      duration: __ENV.DURATION || "30s",
    },
  },
  thresholds: {
    // §8 acceptance target: cache hits return in < 10 ms (gateway overhead only).
    "http_req_duration{expected_response:true}": ["p(99)<10"],
    http_req_failed: ["rate<0.01"],
  },
};

const body = JSON.stringify({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "ping for the gateway load test" }],
});

function headers() {
  return { Authorization: `Bearer ${VK}`, "Content-Type": "application/json" };
}

export function setup() {
  if (!VK) throw new Error("set VK=<virtual key> (and GW if not http://localhost:8080)");
  // One warm-up call fills the cache so the measured run is all hits.
  http.post(`${GW}/v1/chat/completions`, body, { headers: headers() });
}

export default function () {
  const res = http.post(`${GW}/v1/chat/completions`, body, { headers: headers() });
  latency.add(res.timings.duration);
  check(res, {
    "status 200": (r) => r.status === 200,
    "served from cache": (r) => r.headers["X-Cache"] === "HIT" || r.headers["X-Cache"] === "SEMANTIC",
  });
}
