# Contributing to Lintasan

Terima kasih sudah tertarik berkontribusi! 🙏

## Getting Started

```bash
# Fork & clone
git clone https://github.com/YOUR_USERNAME/lintasan.git
cd lintasan

# Install dependencies
npm install

# Initialize database
npm run setup

# Start development server
npm run dev
```

Server berjalan di `http://localhost:20180`

## Development Workflow

1. **Fork** repository ini
2. **Create branch** dari `main`: `git checkout -b feat/your-feature`
3. **Develop** — pastikan `npm run build` berhasil
4. **Commit** dengan conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`
5. **Push** dan buat Pull Request

## Project Structure

```
lintasan/
├── app/                  # Next.js App Router
│   ├── api/              # API routes
│   ├── dashboard/        # Dashboard UI pages
│   └── page.js           # Landing page
├── lib/                  # Core logic
│   ├── db/               # SQLite database
│   ├── providers/        # Provider executors
│   ├── embedding-cache.js
│   ├── router.js
│   └── ...
├── plugins/              # User plugins directory
├── scripts/              # Setup & utility scripts
└── data/                 # SQLite DB (gitignored)
```

## Code Style

- JavaScript (ES Modules) — bukan TypeScript
- `"use client"` untuk semua dashboard pages
- CSS variables dari `app/globals.css` — jangan hardcode warna
- Inline styles (React style objects) — bukan CSS modules
- `credentials: "include"` di semua client-side `fetch()` calls

## Adding a New Feature

1. **API route**: `app/api/your-feature/route.js`
   - Tambahkan `validateDashboardSession` untuk auth
   - Return `{ data: ... }` wrapper
2. **Dashboard page**: `app/dashboard/your-feature/page.js`
   - Follow Tabler-inspired design pattern (lihat existing pages)
   - Include loading skeleton + empty state
3. **Core logic**: `lib/your-feature.js`
4. **Update sidebar**: `app/dashboard/layout.js`

## Testing

```bash
# Build check (must pass)
npm run build

# Manual test
curl http://localhost:20180/api/v1/chat/completions \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}'
```

## Provider Presets

Mau tambah provider baru? Edit `lib/provider-presets.js`:

```javascript
{
  id: "your-provider",
  name: "Your Provider",
  category: "other",
  baseUrl: "https://api.provider.com",
  format: "openai",
  chatPath: "/v1/chat/completions",
  modelsPath: "/v1/models",
  authHeader: "Authorization",
  authPrefix: "Bearer",
}
```

## Plugin Development

Plugins live in `plugins/` directory. Basic structure:

```javascript
export default {
  name: "my-plugin",
  version: "1.0.0",
  hooks: {
    beforeRequest(req) { /* modify request */ return req; },
    afterResponse(res) { /* modify response */ return res; },
    onError(err) { /* handle error */ },
  }
};
```

## Reporting Issues

- Gunakan GitHub Issues
- Sertakan: steps to reproduce, expected vs actual behavior, environment info
- Untuk security issues, email langsung ke maintainer (jangan public issue)

## License

MIT — kontribusi kamu juga akan di-license MIT.
