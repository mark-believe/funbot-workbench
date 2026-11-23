document.querySelectorAll('[data-tab-target]').forEach((button) => {
  button.addEventListener('click', () => {
    document.querySelector(`.tab[data-tab="${button.dataset.tabTarget}"]`)?.click();
  });
});

api('/api/state').then((state) => {
  const firstRun = !state.hasVault;
  document.querySelector('.launchpad').hidden = !firstRun;
  if (firstRun) document.getElementById('secImportHelp').open = true;
}).catch(() => {
  // Leave the starting path visible when the local service is unavailable.
});
