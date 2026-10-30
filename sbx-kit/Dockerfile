# syntax=docker/dockerfile:1.7
# Based on https://github.com/docker/sbx-kits-contrib/blob/main/pi/Dockerfile
ARG BASE_IMAGE=docker/sandbox-templates:shell-docker
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

######################################################################
# Setup
######################################################################
ENV EDITOR=nvim VISUAL=nvim
ARG PI_VERSION=latest
ARG COLGREP_VERSION=1.8.5


######################################################################
# Install packages
######################################################################
USER root
# libatomic1: upstream Node binaries link libatomic.so.1; base image lacks it.
RUN apt-get update && \
    apt-get install -y --no-install-recommends fd-find ripgrep neovim libatomic1 xz-utils jq gh && \
    gh --version && \
    rm -rf /var/lib/apt/lists/* && \
    ln -sf "$(command -v fdfind)" /usr/local/bin/fd


######################################################################
# Optional public CA supplied by bin/build-image; Used by npm/curl
######################################################################
RUN --mount=type=secret,id=custom_ca \
    if [ -f /run/secrets/custom_ca ]; then \
        install -m 0644 /run/secrets/custom_ca \
            /usr/local/share/ca-certificates/custom-ca.crt && \
        update-ca-certificates; \
    fi


######################################################################
# Project configuration: upstream TOML-to-JSON CLI, no Go runtime needed
######################################################################
ARG TARGETARCH
RUN version=2.4.3 && \
    case "$TARGETARCH" in \
        amd64) checksum=70282ec360a01cd6895a01267110338c6dae00d73d0c00a83b5bdc3b1884d083 ;; \
        arm64) checksum=1e5bb8e8f42dded2a811594f0b15e01ab0546d1c2c3b8b388252d3b6e2084ab0 ;; \
        *) echo "Unsupported tomljson architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fLsS --retry 3 \
        "https://github.com/pelletier/go-toml/releases/download/v${version}/tomljson_${version}_linux_${TARGETARCH}.tar.xz" \
        -o /var/tmp/tomljson.tar.xz && \
    printf '%s  %s\n' "$checksum" /var/tmp/tomljson.tar.xz | sha256sum -c - && \
    tar -xJf /var/tmp/tomljson.tar.xz -C /usr/local/bin tomljson && \
    rm /var/tmp/tomljson.tar.xz


######################################################################
# Install mise and its default tools outside shared cache mounts
######################################################################
ENV MISE_DATA_DIR=/opt/pissbox/mise
# Keep Pi and its self-updates independent of the selected Node version.
ENV NPM_CONFIG_PREFIX=/opt/pissbox/npm
ENV NODE_USE_SYSTEM_CA=1
ENV PATH="/opt/pissbox/mise/shims:/opt/pissbox/npm/bin:${PATH}"
RUN version=2026.10.21 && \
    case "$TARGETARCH" in \
        amd64) arch=x64; checksum=e1743bb7628927f93d4ffb2c4e2c4f71186307612f59ebc7f85d52e3a7f42461 ;; \
        arm64) arch=arm64; checksum=3a7ecb96f2d9fbf5cb05631f1da02699d49e592b3ed3adfd5a7fce7420d43ee9 ;; \
        *) echo "Unsupported mise architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fLsS --retry 3 \
        "https://github.com/jdx/mise/releases/download/v${version}/mise-v${version}-linux-${arch}.tar.xz" \
        -o /var/tmp/mise.tar.xz && \
    printf '%s  %s\n' "$checksum" /var/tmp/mise.tar.xz | sha256sum -c - && \
    tar -xJf /var/tmp/mise.tar.xz -C /usr/local/bin --strip-components=2 mise/bin/mise && \
    rm /var/tmp/mise.tar.xz && \
    mkdir -p "$MISE_DATA_DIR" "$NPM_CONFIG_PREFIX" && \
    chown -R agent:agent "$MISE_DATA_DIR" "$NPM_CONFIG_PREFIX" && \
    mise --version

# Shims select project versions in non-interactive shells too; no activation needed.
USER agent
RUN mise use -g node@26.9.0 pnpm@12.15.0 && \
    test "$(node --version)" = "v26.9.0" && \
    test "$(pnpm --version)" = "12.15.0"

######################################################################
# Install pi-coding-agent
######################################################################

# Remote metadata refreshes the install cache; chmod lets agent read it.
USER root
ADD --chmod=644 https://registry.npmjs.org/@earendil-works%2Fpi-coding-agent/${PI_VERSION} /tmp/pi-latest.json

# Install as agent so self-updates retain write access.
USER agent
RUN version="$(node -p 'require("/tmp/pi-latest.json").version')" && \
    npm install -g "@earendil-works/pi-coding-agent@${version}" && \
    pi --offline --version

######################################################################
# Install Colgrep
######################################################################


USER agent
# Unmanaged installation avoids shell-profile edits and shared config data.
RUN curl --proto '=https' --tlsv1.2 -LsSf \
        "https://github.com/lightonai/next-plaid/releases/download/v${COLGREP_VERSION}/colgrep-installer.sh" \
        -o "$HOME/colgrep-installer.sh" && \
    COLGREP_UNMANAGED_INSTALL="$HOME/.local/bin" sh "$HOME/colgrep-installer.sh" && \
    rm "$HOME/colgrep-installer.sh" && \
    "$HOME/.local/bin/colgrep" --version

# Keep the pinned search model outside HOME: shared cache mounts hide image data.
USER root
ARG COLGREP_MODEL_REVISION=4bcdf5ed93f791259eb130b577a240f753d68dd8
RUN model_dir=/opt/pissbox/models/LateOn-Code-edge && \
    mkdir -p "$model_dir" && \
    for file in tokenizer.json config_sentence_transformers.json config.json \
                onnx_config.json model_int8.onnx model.onnx; do \
        curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fLsS --retry 3 \
            "https://huggingface.co/lightonai/LateOn-Code-edge/resolve/${COLGREP_MODEL_REVISION}/${file}" \
            -o "$model_dir/$file" || exit 1; \
    done && \
    chmod -R a+rX "$model_dir"

# Kit setup seeds this into shared state only when no config exists.
RUN printf '%s\n' '{"default_model":"/opt/pissbox/models/LateOn-Code-edge"}' \
        > /opt/pissbox/colgrep-config.json



##########################################################
# Keep tools available in minimal-PATH execution contexts.
##########################################################
RUN ln -sf "$(npm prefix -g)/bin/pi" /usr/local/bin/pi && \
    ln -sf /home/agent/.local/bin/colgrep /usr/local/bin/colgrep && \
    for tool in node npm npx pnpm pnpx; do \
        ln -sf "/opt/pissbox/mise/shims/$tool" "/usr/local/bin/$tool" || exit 1; \
    done

LABEL com.docker.sandboxes.start-docker="true"
LABEL com.docker.sandboxes.flavor="pi"
LABEL com.docker.sandboxes.base="${BASE_IMAGE}"

USER agent
WORKDIR /home/agent
CMD ["pi --offline"]
