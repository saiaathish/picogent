(() => {
  const form = document.querySelector("#profile-form");
  const input = document.querySelector("#display-name");
  const status = document.querySelector("#status");

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    status.textContent = `Saved ${input.value.trim()}.`;
  });
})();
