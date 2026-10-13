# Linux audio players with null outputs, so `jingle play --wait` really runs
# each player with jingle's exact flags. Nothing is audible. See smoke.sh.
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq \
 && apt-get install -y -qq --no-install-recommends \
      pulseaudio pulseaudio-utils alsa-utils ffmpeg \
      pipewire pipewire-bin wireplumber dbus dbus-user-session sox \
 && rm -rf /var/lib/apt/lists/* \
 && useradd -m u
USER u
WORKDIR /home/u
ENTRYPOINT ["sh", "/smoke/smoke.sh"]
