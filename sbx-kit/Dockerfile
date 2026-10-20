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
RUN apt-get update && \
    apt-get install -y --no-install-recommends fd-find ripgrep neovim xz-utils && \
    rm -rf /var/lib/apt/lists/* && \
    ln -sf "$(command -v fdfind)" /usr/local/bin/fd


# ######################################################################
# # Uncomment this block if you need to install your own ca-certificate
# ######################################################################
# USER root
# COPY ca.crt /usr/local/share/ca-certificates/ca.crt
# RUN update-ca-certificates


######################################################################
# Install pi-coding-agent
######################################################################

# Remote metadata refreshes the install cache; chmod lets agent read it.
USER root
ADD --chmod=644 https://registry.npmjs.org/@earendil-works%2Fpi-coding-agent/${PI_VERSION} /tmp/pi-latest.json

# Install as agent so self-updates retain write access.
USER agent
ENV NODE_USE_SYSTEM_CA=1
RUN version="$(node -p 'require("/tmp/pi-latest.json").version')" && \
	npm install -g pnpm && \
    npm install -g "@earendil-works/pi-coding-agent@${version}" && \
    pi --offline --version

######################################################################
# Install Colgrep
######################################################################


USER root
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
    ln -sf /home/agent/.local/bin/colgrep /usr/local/bin/colgrep

LABEL com.docker.sandboxes.start-docker="true"
LABEL com.docker.sandboxes.flavor="pi"
LABEL com.docker.sandboxes.base="${BASE_IMAGE}"

USER agent
WORKDIR /home/agent
CMD ["pi --offline"]
