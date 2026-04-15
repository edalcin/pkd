/**
 * share-view.js — Public share page.
 * Minimal: no editor, no fetch calls, just image zoom on click.
 */
document.addEventListener('DOMContentLoaded', () => {
  // Image click-to-zoom
  document.querySelectorAll('#share-body img').forEach(img => {
    img.style.cursor = 'zoom-in';
    img.addEventListener('click', () => {
      const overlay = document.createElement('div');
      overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.85);display:flex;align-items:center;justify-content:center;z-index:1000;cursor:zoom-out';
      const clone = img.cloneNode();
      clone.style.cssText = 'max-width:90vw;max-height:90vh;object-fit:contain';
      overlay.appendChild(clone);
      overlay.addEventListener('click', () => overlay.remove());
      document.body.appendChild(overlay);
    });
  });
});
