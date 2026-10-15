// Runs in <head>, before the page paints: apply this browser's text size and
// theme, else the config default the server put on <html data-text-size>.
(function () {
  var d = document.documentElement;
  var size = d.getAttribute("data-text-size") || "16";
  try {
    var saved = localStorage.getItem("mrm.textSize");
    if (saved) size = saved;
    var theme = localStorage.getItem("mrm.theme");
    if (theme === "light" || theme === "dark") d.setAttribute("data-theme", theme);
  } catch (e) { /* storage blocked: use the defaults */ }
  d.style.fontSize = size + "px";
})();
