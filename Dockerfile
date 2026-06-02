# Frontend image: Express static server (public/) + /api reverse-proxy to the
# coordinator. The proxy target is COORDINATOR_URL, set by the k8s manifest to the
# in-cluster coordinator service (no host port-forward needed).
#
# Build from the repo root:  docker build -t cohort-frontend:v1.0.0 .

FROM node:24-slim AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY tsconfig.json ./
COPY src ./src
# Emit to dist/ explicitly (tsconfig sets no outDir — it's a typecheck-only config).
RUN npx tsc --outDir dist && ls dist

FROM node:24-slim AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY package.json package-lock.json* ./
RUN npm install --omit=dev   # express only (no tsx/typescript at runtime)
COPY --from=build /app/dist ./dist
COPY public ./public
EXPOSE 3000
# dist/server.js resolves ../public -> /app/public. COORDINATOR_URL comes from env.
CMD ["node", "dist/server.js"]
