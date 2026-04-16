<script>
  import { login } from '../stores/auth.js'

  let password = $state('')
  let error = $state('')
  let loading = $state(false)

  async function handleSubmit(e) {
    e.preventDefault()
    error = ''
    loading = true
    try {
      const { ok, status } = await login(password)
      if (!ok) {
        if (status === 429) {
          error = 'Muitas tentativas. Aguarde 30 minutos.'
        } else {
          error = 'Senha incorreta.'
        }
        password = ''
      }
    } catch {
      error = 'Erro de conexão. Tente novamente.'
    } finally {
      loading = false
    }
  }
</script>

<div class="login-wrap">
  <div class="login-card">
    <div class="login-logo">🔒</div>
    <h1>PKD</h1>
    <p class="subtitle">Personal Knowledge Database</p>

    <form onsubmit={handleSubmit}>
      <label for="password">Senha mestra</label>
      <input
        id="password"
        type="password"
        bind:value={password}
        placeholder="Digite sua senha"
        autocomplete="current-password"
        required
        disabled={loading}
      />
      <button type="submit" class="btn btn-primary submit-btn" disabled={loading}>
        {#if loading}
          <span class="spinner-sm"></span> Entrando…
        {:else}
          Entrar
        {/if}
      </button>
      {#if error}
        <p class="error-msg" role="alert">{error}</p>
      {/if}
    </form>
  </div>
</div>

<style>
  .login-wrap {
    min-height: 100dvh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    padding: 1rem;
  }

  .login-card {
    background: var(--bg-panel);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    padding: 2.5rem 2rem;
    width: min(380px, 100%);
    text-align: center;
  }

  .login-logo {
    font-size: 2.5rem;
    margin-bottom: .5rem;
  }

  h1 {
    font-size: 1.75rem;
    font-weight: 700;
    letter-spacing: -.5px;
    margin-bottom: .25rem;
  }

  .subtitle {
    font-size: .875rem;
    color: var(--text-muted);
    margin-bottom: 1.75rem;
  }

  label {
    display: block;
    text-align: left;
    font-size: .875rem;
    color: var(--text-muted);
    margin-bottom: .375rem;
  }

  input[type="password"] {
    width: 100%;
    padding: .65rem .875rem;
    margin-bottom: 1rem;
    font-size: 1rem;
  }

  .submit-btn {
    width: 100%;
    justify-content: center;
    padding: .7rem;
    font-size: 1rem;
  }

  .error-msg {
    color: var(--danger);
    font-size: .875rem;
    margin-top: .75rem;
  }

  .spinner-sm {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid rgba(255,255,255,.4);
    border-top-color: #fff;
    border-radius: 50%;
    animation: spin .7s linear infinite;
  }

  @keyframes spin { to { transform: rotate(360deg); } }
</style>
