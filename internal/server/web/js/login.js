// login.js — script de autenticação da página de login
// Carregado como arquivo externo para ser permitido pelo CSP (script-src 'self').

(function () {
  const form    = document.getElementById('login-form');
  const errMsg  = document.getElementById('error-msg');
  const passInput = document.getElementById('password');

  // Extrai o token CSRF do cookie pkd_csrf, set pelo servidor no GET /login.
  function getCsrfToken() {
    const entry = document.cookie.split(';')
      .map(c => c.trim())
      .find(c => c.startsWith('pkd_csrf='));
    return entry ? entry.slice('pkd_csrf='.length) : '';
  }

  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    errMsg.textContent = '';

    const password   = passInput.value;
    const csrfToken  = getCsrfToken();

    let res;
    try {
      res = await fetch('/api/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': csrfToken,
        },
        body: JSON.stringify({ password }),
      });
    } catch (networkErr) {
      errMsg.textContent = 'Erro de rede. Verifique a conexão.';
      return;
    }

    if (res.ok) {
      window.location.replace('/');
    } else if (res.status === 429) {
      errMsg.textContent = 'Muitas tentativas incorretas. Aguarde 30 minutos.';
    } else if (res.status === 403) {
      // CSRF inválido — recarrega a página para obter um novo cookie
      window.location.reload();
    } else {
      errMsg.textContent = 'Senha incorreta.';
    }
  });
}());
