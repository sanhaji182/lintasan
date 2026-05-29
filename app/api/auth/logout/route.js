export const runtime = "nodejs";

export async function POST() {
  return Response.json({ success: true }, {
    headers: {
      "Set-Cookie": "sr_session=; Path=/; HttpOnly; Max-Age=0",
    },
  });
}
