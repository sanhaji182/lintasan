# ─── Build Stage ───────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

# Install build dependencies for better-sqlite3
RUN apk add --no-cache python3 make g++

# Install dependencies
COPY package.json package-lock.json* ./
RUN npm ci

# Copy source and build
COPY . .
RUN npm run build

# ─── Production Stage ─────────────────────────
FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV PORT=20180

# Install runtime dependencies for better-sqlite3
RUN apk add --no-cache python3 make g++

# Create non-root user
RUN addgroup --system --gid 1001 lintasan && \
    adduser --system --uid 1001 lintasan

# Copy app files
COPY --from=builder /app/package.json ./
COPY --from=builder /app/package-lock.json* ./
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
COPY --from=builder /app/lib ./lib
COPY --from=builder /app/app ./app
COPY --from=builder /app/scripts ./scripts
COPY --from=builder /app/plugins ./plugins
COPY --from=builder /app/next.config.mjs ./

# Create data directory for SQLite
RUN mkdir -p /app/data /app/data/backups && chown -R lintasan:lintasan /app/data

VOLUME /app/data

USER lintasan

EXPOSE 20180

# Initialize DB and start
CMD ["sh", "-c", "node scripts/seed.js 2>/dev/null; npx next start -p ${PORT}"]
