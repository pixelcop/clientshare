<template>
  <p class="landing-wordmark text-slate-950">
    <span>Client</span><span class="landing-wordmark-accent">Share</span>
  </p>
</template>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Sora:wght@600;700;800&display=swap');
.landing-wordmark {
  --font-wordmark: 'Sora', 'Avenir Next', 'Segoe UI', sans-serif;
  font-family: var(--font-wordmark);
  font-size: clamp(1.45rem, 1.2rem + 0.9vw, 2rem);
  font-weight: 700;
  letter-spacing: -0.09em;
  span:first-child {
    letter-spacing: -0.07em;
  }
  line-height: 0.88;
  margin: 0;
  text-wrap: nowrap;

  .landing-wordmark-accent {
    background: linear-gradient(
      135deg,
      rgb(29 78 216) 0%,
      rgb(14 165 233) 45%,
      rgb(244 114 182) 100%
    );
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
    padding-right: 0.1em;
    margin-right: -0.1em;
  }
}
</style>
