# syntax=docker/dockerfile:1.7
ARG BASE_IMAGE=docker/sandbox-templates:shell-docker
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER root
RUN apt-get update && \
    apt-get install -y --no-install-recommends fd-find ripgrep neovim && \
    rm -rf /var/lib/apt/lists/* && \
    ln -sf "$(command -v fdfind)" /usr/local/bin/fd

ENV EDITOR=nvim VISUAL=nvim

ARG PI_VERSION=latest

# Remote metadata refreshes the install cache; chmod lets agent read it.
USER root
ADD --chmod=644 https://registry.npmjs.org/@earendil-works%2Fpi-coding-agent/${PI_VERSION} /tmp/pi-latest.json

# Install as agent so self-updates retain write access.
USER agent
RUN version="$(node -p 'require("/tmp/pi-latest.json").version')" && \
    npm install -g "@earendil-works/pi-coding-agent@${version}" && \
    npm install -g pnpm && \
    pi --version

# Keep pi available in minimal-PATH execution contexts.
USER root
RUN ln -sf "$(npm prefix -g)/bin/pi" /usr/local/bin/pi

LABEL com.docker.sandboxes.start-docker="true"
LABEL com.docker.sandboxes.flavor="pi"
LABEL com.docker.sandboxes.base="${BASE_IMAGE}"

USER agent
WORKDIR /home/agent
CMD ["pi"]
