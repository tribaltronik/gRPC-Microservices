import grpc from "k6/net/grpc";

const cert = open("../certs/api-gateway/cert.pem");
const key = open("../certs/api-gateway/key.pem");
const ca = open("../certs/ca/ca.pem");

export default function() {
  const c = new grpc.Client();
  try {
    c.connect("localhost:50051", {
      tls: { cert: cert, key: key, ca: [ca] },
    });
    const resp = c.invoke("grpc.health.v1.Health/Check", {});
    console.log("health:", JSON.stringify(resp.message));
    c.close();
  } catch(e) {
    console.log("Error:", e.message);
  }
}
