// jingle landing page: entrance and scroll motion (GSAP), the copy button,
// and the "Plays on" pills that play jingle's built-in sounds.
(function () {
  "use strict";

  const root = document.documentElement;
  const live = document.getElementById("live");

  function announce(msg) {
    live.textContent = "";
    // A fresh text node makes screen readers repeat identical messages.
    requestAnimationFrame(() => { live.textContent = msg; });
  }

  // ---------- Motion ----------

  const hasGSAP = typeof window.gsap !== "undefined";

  if (!hasGSAP) {
    root.classList.remove("js");
  } else {
    window.__jingleAnim = true;
    gsap.registerPlugin(ScrollTrigger, SplitText);

    const mm = gsap.matchMedia();

    mm.add(
      {
        motion: "(prefers-reduced-motion: no-preference)",
        reduce: "(prefers-reduced-motion: reduce)",
        stacked: "(max-width: 767px)",
      },
      (ctx) => {
        if (ctx.conditions.reduce) {
          root.classList.remove("js");
          return;
        }
        root.classList.add("js");
        intro();
        scroll(ctx.conditions.stacked);
      }
    );
  }

  function intro() {
    const heroImg = document.querySelector(".hero__media img");
    gsap.set(heroImg, { scale: 1.12 }); // headroom for the scroll parallax

    const tl = gsap.timeline({ defaults: { ease: "expo.out" } });

    // 1. Nav
    tl.from(".nav .js-anim", { autoAlpha: 0, y: -20, duration: 1, stagger: 0.08 }, 0);

    // 2. Image wipes up from the bottom while it settles.
    tl.fromTo(
      ".hero__media",
      { autoAlpha: 1, clipPath: "inset(100% 0% 0% 0%)" },
      { clipPath: "inset(0% 0% 0% 0%)", duration: 1.4, ease: "expo.inOut", clearProps: "clipPath" },
      0.1
    );
    tl.from(heroImg, { scale: 1.4, duration: 2, ease: "expo.out" }, 0.1);

    // 3. Headline: words rise out of masked lines. autoSplit re-splits on
    // resize and font swap, keeping the tween's progress.
    SplitText.create(".hero__title", {
      type: "lines,words",
      mask: "lines",
      autoSplit: true,
      onSplit(self) {
        gsap.set(self.elements, { autoAlpha: 1 });
        return gsap.from(self.words, {
          yPercent: 110,
          duration: 1.2,
          stagger: 0.06,
          ease: "expo.out",
          delay: 0.5,
        });
      },
    });

    // 4. The rest of the column, in reading order.
    tl.from(".hero__sub", { autoAlpha: 0, y: 16, duration: 1 }, 0.95)
      .from(".codecard", { autoAlpha: 0, y: 32, duration: 1.1 }, 1.05)
      .from(".divider", { autoAlpha: 0, duration: 0.4 }, 1.2)
      .from(".divider__rule", {
        scaleX: 0,
        transformOrigin: (i) => (i === 0 ? "100% 50%" : "0% 50%"),
        duration: 1.1,
      }, 1.2)
      .from(".divider__or", { y: 10, duration: 0.8 }, 1.25)
      .from(".hero__cta", { autoAlpha: 0, y: 16, scale: 0.92, duration: 1 }, 1.35)
      .from(".hero__note", { autoAlpha: 0, duration: 1.2, ease: "power2.out" }, 1.5);
  }

  // stacked: the Plays on panel is a column (phones), so the pills sit well
  // below the panel top. Each part triggers off its own position.
  function scroll(stacked) {
    // Gentle parallax on the hero photo.
    gsap.to(".hero__media img", {
      yPercent: 5,
      ease: "none",
      scrollTrigger: { trigger: ".hero", start: "top top", end: "bottom top", scrub: true },
    });

    // The ink panel opens up to full size as it comes in.
    gsap.fromTo(
      ".plays__panel",
      { scale: 0.9, borderRadius: 48 },
      {
        scale: 1,
        borderRadius: 0,
        ease: "none",
        scrollTrigger: { trigger: ".plays", start: "top bottom", end: "top 15%", scrub: 0.6 },
      }
    );

    SplitText.create(".plays__title", {
      type: "chars",
      mask: "chars",
      autoSplit: true,
      onSplit(self) {
        gsap.set(self.elements, { autoAlpha: 1 });
        return gsap.from(self.chars, {
          yPercent: 110,
          duration: 1,
          stagger: 0.035,
          ease: "expo.out",
          scrollTrigger: { trigger: ".plays__title", start: "top 85%" },
        });
      },
    });

    gsap.from(".plays__list .js-anim", {
      autoAlpha: 0,
      x: stacked ? 0 : 60,
      y: stacked ? 32 : 0,
      duration: 1.1,
      stagger: 0.12,
      ease: "expo.out",
      scrollTrigger: { trigger: ".plays__list", start: stacked ? "top 90%" : "top 80%" },
    });
  }

  function motionOK() {
    return hasGSAP && !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  // ---------- Copy ----------

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.setAttribute("readonly", "");
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      let ok = false;
      try { ok = document.execCommand("copy"); } catch { ok = false; }
      ta.remove();
      return ok;
    }
  }

  document.querySelectorAll("[data-copy]").forEach((btn) => {
    let timer;
    btn.addEventListener("click", async () => {
      const src = document.querySelector(btn.dataset.copy);
      if (!src || !(await copyText(src.textContent.trim()))) {
        announce("Copy failed. Select the commands and copy them by hand.");
        return;
      }
      btn.classList.add("is-copied");
      announce("Copied to clipboard");
      clearTimeout(timer);
      timer = setTimeout(() => btn.classList.remove("is-copied"), 1600);
    });
  });

  // ---------- Sounds ----------

  const audios = new Map(); // pill -> HTMLAudioElement
  let playing = null;

  function audioFor(pill) {
    let a = audios.get(pill);
    if (!a) {
      a = new Audio(pill.dataset.sound);
      a.preload = "auto";
      a.addEventListener("ended", () => stop(pill));
      a.addEventListener("pause", () => stop(pill));
      audios.set(pill, a);
    }
    return a;
  }

  function beat(pill) {
    const ring = document.createElement("span");
    ring.className = "pill__ring";
    ring.setAttribute("aria-hidden", "true");
    pill.appendChild(ring);
    gsap.fromTo(
      ring,
      { inset: 0, opacity: 0.9 },
      { inset: -16, opacity: 0, duration: 1, ease: "expo.out", onComplete: () => ring.remove() }
    );
    gsap.fromTo(
      pill.querySelector(".pill__icon"),
      { y: -4, rotation: -14 },
      { y: 0, rotation: 0, duration: 0.6, ease: "expo.out" }
    );
  }

  function start(pill) {
    pill.classList.add("is-playing");
    if (!motionOK()) return;
    beat(pill);
    pill._beat = setInterval(() => beat(pill), 420);
  }

  function stop(pill) {
    pill.classList.remove("is-playing");
    clearInterval(pill._beat);
    pill._beat = null;
    if (playing === pill) playing = null;
  }

  document.querySelectorAll(".pill[data-sound]").forEach((pill) => {
    // Start fetching on intent, so the click plays without a gap.
    pill.addEventListener("pointerenter", () => audioFor(pill), { once: true });
    pill.addEventListener("focus", () => audioFor(pill), { once: true });

    pill.addEventListener("click", () => {
      if (playing && playing !== pill) audios.get(playing).pause();
      const a = audioFor(pill);
      stop(pill);
      a.currentTime = 0;
      a.play().then(
        () => { playing = pill; start(pill); },
        () => stop(pill)
      );
    });
  });
})();
