package main

const htmlTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'">
<title>triallang / computed examples</title>
<style>
:root { color-scheme: light; font-family: system-ui, sans-serif; color: #0f172a; background: #f1f5f9; }
* { box-sizing: border-box; }
body { margin: 0; }
main { max-width: 1200px; margin: auto; padding: 40px 24px; }
header { margin-bottom: 28px; }
h1 { margin: 0 0 12px; font-size: clamp(25px, 4vw, 36px); letter-spacing: -.04em; }
header p, footer p { max-width: 78ch; color: #475569; line-height: 1.65; }
a { color: #075985; text-underline-offset: 3px; }
a:hover { color: #0c4a6e; }
.gallery { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px; }
.card { min-width: 0; padding: 20px; border: 1px solid #cbd5e1; border-radius: 12px; background: #fff; }
h2 { margin: 0 0 8px; font-size: 21px; }
.description { margin: 0 0 18px; color: #475569; min-height: 2.8em; line-height: 1.4; }
.screen { display: flex; height: 290px; padding: 16px; overflow: auto; border-radius: 8px; background: #0f172a; color: #d1fae5; }
pre { flex: 0 0 auto; width: max-content; margin: auto; font: 13px/1.2 'Courier New', monospace; tab-size: 4; }
.controls { margin-top: 12px; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.controls[hidden] { display: none; }
button { background: #f8fafc; color: #0f172a; border: 1px solid #94a3b8; border-radius: 5px; padding: 6px 12px; font: inherit; font-size: 13px; cursor: pointer; }
button:hover { background: #e2e8f0; }
button:focus-visible, a:focus-visible, .screen:focus-visible { outline: 3px solid #0284c7; outline-offset: 3px; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; border: 0; }
output { margin-left: auto; color: #475569; font-size: 13px; }
.source { margin: 15px 0 0; font-size: 13px; }
.input { color: #475569; font-size: 13px; }
code { font: .95em 'Courier New', monospace; overflow-wrap: anywhere; }
footer { margin-top: 28px; }
@media (max-width: 850px) { .gallery { grid-template-columns: 1fr; } .description { min-height: 0; } }
@media (max-width: 450px) { main { padding: 28px 12px; } .card { padding: 14px; } .screen { padding: 8px; } pre { font-size: 10px; } }
</style>
</head>
<body>
<main>
<header>
<h1>triallang / computed examples</h1>
<p>Six small programs, executed in triallang. Every preview below comes from an actual run that passes its deposition. The gallery works offline.</p>
<p>The shaders use integer arithmetic on the CPU. Read the <a href="shaders.md">shader guide</a> or the <a href="toys.md">toy guide</a> to run and change them.</p>
</header>
<section class="gallery" aria-label="Program output">
{{range $i, $item := .}}<article class="card" data-example="{{$i}}">
<h2>{{$item.Title}}</h2>
<p class="description">{{$item.Description}}</p>
<div class="screen" tabindex="0" role="region" aria-label="{{$item.Title}} output"><pre id="gallery-frame-{{$i}}"><span>{{index $item.Frames 0}}</span></pre></div>
{{if $item.Animate}}<div class="controls" hidden>
<button type="button" data-action="play" aria-controls="gallery-frame-{{$i}}">Play</button>
<button type="button" data-action="step" aria-controls="gallery-frame-{{$i}}">Step</button>
<button type="button" data-action="reset" aria-controls="gallery-frame-{{$i}}">Reset</button>
<output aria-label="Current frame" aria-live="off">Frame 1 of {{len $item.Frames}}</output>
<span class="sr-only" role="status" aria-live="polite"></span>
</div><noscript><p>Enable JavaScript to play the {{len $item.Frames}} wave frames. The first frame appears above.</p></noscript>{{end}}
{{if $item.Input}}<p class="input">Input: <code>{{$item.Input}}</code></p>{{end}}
<p class="source"><a href="../examples/{{$item.Name}}.trial">Source</a> · <a href="../examples/{{$item.Name}}.deposition">Deposition</a></p>
</article>
{{end}}</section>
<footer>
<p>A deposition states the required output and final records. The generator runs each source and refuses any failed deposition. The wave controls play the four computed frames. They do not run the interpreter in your browser.</p>
<p>From the repository root, run <code>go run ./tools/examplegallery -write -root .</code> to rebuild this gallery. Use <code>-check</code> to make sure that both generated files match the current programs.</p>
</footer>
</main>
<script type="application/json" id="gallery-data">{{.}}</script>
<script>
'use strict';
const gallery = JSON.parse(document.getElementById('gallery-data').textContent);
for (const card of document.querySelectorAll('[data-example]')) {
  const item = gallery[Number(card.dataset.example)];
  if (!item.Animate) continue;
  const controls = card.querySelector('.controls');
  const screen = card.querySelector('pre');
  const play = card.querySelector('[data-action="play"]');
  const counter = card.querySelector('output');
  const status = card.querySelector('[role="status"]');
  let current = 0;
  let timer = null;
  const show = () => {
    screen.textContent = item.Frames[current];
    counter.textContent = 'Frame ' + (current + 1) + ' of ' + item.Frames.length;
  };
  const stop = () => {
    clearInterval(timer);
    timer = null;
    play.textContent = 'Play';
  };
  play.addEventListener('click', () => {
    if (timer !== null) {
      stop(); status.textContent = 'Paused. ' + counter.textContent; return;
    }
    play.textContent = 'Pause';
    status.textContent = 'Playing ' + item.Frames.length + ' frames.';
    timer = setInterval(() => { current = (current + 1) % item.Frames.length; show(); }, 500);
  });
  card.querySelector('[data-action="step"]').addEventListener('click', () => {
    stop(); current = (current + 1) % item.Frames.length; show(); status.textContent = counter.textContent;
  });
  card.querySelector('[data-action="reset"]').addEventListener('click', () => {
    stop(); current = 0; show(); status.textContent = counter.textContent;
  });
  document.addEventListener('visibilitychange', () => { if (document.hidden) stop(); });
  controls.hidden = false;
}
</script>
</body>
</html>
`
