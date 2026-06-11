# syntax=docker/dockerfile:1.7

FROM node:26-alpine3.22@sha256:df14c236fa02ce2e232f234d571ce831c6b67c27605ece534164822257ed256e AS deps

WORKDIR /src

COPY package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund


FROM node:26-alpine3.22@sha256:df14c236fa02ce2e232f234d571ce831c6b67c27605ece534164822257ed256e AS dev

WORKDIR /src

COPY --from=deps /src/node_modules ./node_modules
COPY . .

EXPOSE 5173

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5173"]


FROM node:26-alpine3.22@sha256:df14c236fa02ce2e232f234d571ce831c6b67c27605ece534164822257ed256e AS build

WORKDIR /src

COPY --from=deps /src/node_modules ./node_modules
COPY . .
ARG VITE_API_URL=""
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build


FROM nginxinc/nginx-unprivileged:1.27-alpine@sha256:65e3e85dbaed8ba248841d9d58a899b6197106c23cb0ff1a132b7bfe0547e4c0 AS runtime

COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

USER 101:101

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
