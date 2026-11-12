// Liveness of the web process for the container healthcheck. It does not
// call the API: a backend outage must not restart the frontend.
export const dynamic = "force-dynamic";

export function GET() {
  return Response.json({ status: "ok" });
}
