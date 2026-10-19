import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  ConfigProvider,
  Form,
  Input,
  Layout,
  Menu,
  Popover,
  Space,
  Spin,
  message,
} from 'antd';
import {
  KeyOutlined,
  SafetyCertificateOutlined,
  LockOutlined,
  MoonFilled,
  MoonOutlined,
  SunOutlined,
  TranslationOutlined,
  UserOutlined,
} from '@ant-design/icons';

import { FormProvider, useForm } from 'react-hook-form';
import { HttpUtil, LanguageManager } from '@/utils';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { setMessageInstance } from '@/utils/messageBus';
import SponsorSlot from '@/components/sponsor/SponsorSlot';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import {
  CaptchaAnswerSchema,
  TgCodeSchema,
  LoginFormSchema,
  TwoFactorCodeSchema,
  type LoginFormValues,
} from '@/schemas/login';
import { BrandGlyph, BrandMark } from '@/components/ui';
import { getPasskey, passkeysSupported, type RequestOptionsJSON } from '@/lib/passkey';
import { DEFAULT_BRAND_NAME, panelBrand } from '@/lib/brand/brand';
import './LoginPage.css';

const HEADLINE_INTERVAL_MS = 2000;

type LoginForm = LoginFormValues;

const basePath = window.X_UI_BASE_PATH || '';
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

interface PasskeyPrompt {
  passkeyRequired?: boolean;
  passkeyOptions?: RequestOptionsJSON;
  passkeyChallengeId?: string;
}

export default function LoginPage() {
  const { name: customName, logo } = panelBrand();
  const brandName = customName || DEFAULT_BRAND_NAME;
  const { t } = useTranslation();
  const { isDark, isUltra, toggleTheme, toggleUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const [fetched, setFetched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [twoFactorEnable, setTwoFactorEnable] = useState(false);
  const [tgCodeRequired, setTgCodeRequired] = useState(false);
  const [passkeyLoginEnabled, setPasskeyLoginEnabled] = useState(false);
  const [captcha, setCaptcha] = useState<{ id: string; image: string } | null>(null);
  const [headlineIndex, setHeadlineIndex] = useState(0);
  const methods = useForm<LoginForm>({
    defaultValues: {
      username: '',
      password: '',
      twoFactorCode: '',
      captchaAnswer: '',
      tgCode: '',
    },
  });
  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage());

  const headlineWords = useMemo(() => [t('pages.login.hello'), t('pages.login.title')], [t]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setHeadlineIndex((i) => (i + 1) % headlineWords.length);
    }, HEADLINE_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [headlineWords.length]);

  const loadCaptcha = useCallback(async () => {
    const msg = await HttpUtil.get('/captcha', undefined, { silent: true });
    const obj = msg?.obj as { enabled?: boolean; id?: string; image?: string } | undefined;
    setCaptcha(obj?.enabled && obj.id && obj.image ? { id: obj.id, image: obj.image } : null);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      await loadCaptcha();
      const msg = await HttpUtil.post('/getTwoFactorEnable');
      if (cancelled) return;
      if (msg.success) setTwoFactorEnable(!!msg.obj);
      if (passkeysSupported()) {
        const status = await HttpUtil.post('/passkey/login/status', undefined, { silent: true });
        if (!cancelled && status?.success) setPasskeyLoginEnabled(!!status.obj);
      }
      setFetched(true);
    })();
    return () => {
      cancelled = true;
    };
  }, [loadCaptcha]);

  const onSubmit = useCallback(
    async (values: LoginForm) => {
      setSubmitting(true);
      try {
        const base = { ...values, captchaId: captcha?.id ?? '' };
        let msg = await HttpUtil.post('/login', base);
        const prompt = msg.obj as PasskeyPrompt | null;
        if (!msg.success && prompt?.passkeyRequired && prompt.passkeyOptions) {
          try {
            const credential = await getPasskey(prompt.passkeyOptions);
            msg = await HttpUtil.post('/login', {
              ...base,
              passkeyChallengeId: prompt.passkeyChallengeId ?? '',
              passkeyCredential: JSON.stringify(credential),
            });
          } catch {
            messageApi.error(t('pages.login.toasts.passkeyFailed'));
            if (captcha) await loadCaptcha();
            return;
          }
        }
        if (msg.success) window.location.href = basePath + 'panel/';
        else if ((msg.obj as { tgCodeRequired?: boolean } | null)?.tgCodeRequired) {
          setTgCodeRequired(true);
          if (captcha) await loadCaptcha();
        } else if (captcha) {
          methods.setValue('captchaAnswer', '');
          await loadCaptcha();
        }
      } finally {
        setSubmitting(false);
      }
    },
    [captcha, loadCaptcha, methods, messageApi, t],
  );

  const onPasskeyLogin = useCallback(async () => {
    const twoFactorCode = methods.getValues('twoFactorCode') ?? '';
    setSubmitting(true);
    try {
      const begin = await HttpUtil.post('/passkey/login/begin');
      const obj = begin?.obj as { options?: RequestOptionsJSON; challengeId?: string } | null;
      if (!begin?.success || !obj?.options) return;
      let credential;
      try {
        credential = await getPasskey(obj.options);
      } catch {
        messageApi.error(t('pages.login.toasts.passkeyFailed'));
        return;
      }
      const msg = await HttpUtil.post(
        '/passkey/login/finish',
        { challengeId: obj.challengeId, credential, twoFactorCode },
        JSON_HEADERS,
      );
      if (msg.success) window.location.href = basePath + 'panel/';
    } finally {
      setSubmitting(false);
    }
  }, [methods, messageApi, t]);

  const onLangChange = useCallback((next: string) => {
    setLang(next);
    LanguageManager.setLanguage(next);
  }, []);

  const cycleTheme = useCallback(() => {
    pauseAnimationsUntilLeave('login-theme-cycle');
    if (!isDark) {
      toggleTheme();
      if (isUltra) toggleUltra();
    } else if (!isUltra) {
      toggleUltra();
    } else {
      toggleUltra();
      toggleTheme();
    }
  }, [isDark, isUltra, toggleTheme, toggleUltra]);

  const pageClass = useMemo(() => {
    const classes = ['login-app'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const langMenuItems = useMemo(
    () =>
      (LanguageManager.supportedLanguages as { value: string; name: string; icon: string }[]).map(
        (l) => ({
          key: l.value,
          label: (
            <Space size={8}>
              <span aria-hidden="true">{l.icon}</span>
              <span>{l.name}</span>
            </Space>
          ),
        }),
      ),
    [],
  );

  const themeIcon = !isDark ? <SunOutlined /> : !isUltra ? <MoonOutlined /> : <MoonFilled />;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <Layout.Content className="login-content">
          <div className="login-toolbar">
            <Button
              id="login-theme-cycle"
              shape="circle"
              size="large"
              className="toolbar-btn"
              aria-label={t('menu.theme')}
              title={t('menu.theme')}
              icon={themeIcon}
              onClick={cycleTheme}
            />
            <Popover
              rootClassName={isDark ? 'dark' : 'light'}
              placement="bottomRight"
              trigger="click"
              styles={{ content: { padding: 4 } }}
              content={
                <Menu
                  mode="vertical"
                  selectable
                  selectedKeys={[lang]}
                  items={langMenuItems}
                  onClick={({ key }) => onLangChange(key)}
                  style={{ border: 'none', minWidth: 160 }}
                />
              }
            >
              <Button
                shape="circle"
                size="large"
                className="toolbar-btn"
                aria-label={t('pages.settings.language')}
                icon={<TranslationOutlined />}
              />
            </Popover>
          </div>

          <div className="login-split">
            <aside className="login-aside" aria-hidden="true">
              <div className="login-aside-rings" />
              <div className="login-aside-brand">
                {logo ? (
                  <img className="login-aside-logo" src={logo} alt="" />
                ) : (
                  <BrandGlyph bare size={260} className="login-aside-glyph" />
                )}
                <span className="login-aside-name" dir="auto">
                  {brandName}
                </span>
                <span className="login-aside-accent" />
              </div>
            </aside>
            <div className="login-wrapper">
              {!fetched ? (
                <div className="login-loading">
                  <Spin size="large" />
                </div>
              ) : (
                <div className="login-card">
                  <div className="brand">
                    <BrandMark className="brand-name" />
                  </div>
                  <h2 className="welcome">
                    <b key={headlineIndex}>{headlineWords[headlineIndex]}</b>
                  </h2>

                  <FormProvider {...methods}>
                    <Form
                      layout="vertical"
                      className="login-form"
                      onFinish={methods.handleSubmit(onSubmit)}
                    >
                      <FormField
                        name="username"
                        label={t('username')}
                        rules={{ validate: rhfZodValidate(LoginFormSchema.shape.username) }}
                      >
                        <Input
                          prefix={<UserOutlined />}
                          autoComplete="username"
                          size="large"
                          placeholder={t('username')}
                          autoFocus
                        />
                      </FormField>

                      <FormField
                        name="password"
                        label={t('password')}
                        rules={{ validate: rhfZodValidate(LoginFormSchema.shape.password) }}
                      >
                        <Input.Password
                          prefix={<LockOutlined />}
                          autoComplete="current-password"
                          size="large"
                          placeholder={t('password')}
                        />
                      </FormField>

                      {twoFactorEnable && (
                        <FormField
                          name="twoFactorCode"
                          label={t('twoFactorCode')}
                          rules={{ validate: rhfZodValidate(TwoFactorCodeSchema) }}
                        >
                          <Input
                            prefix={<KeyOutlined />}
                            autoComplete="one-time-code"
                            size="large"
                            placeholder={t('twoFactorCode')}
                          />
                        </FormField>
                      )}

                      {tgCodeRequired && (
                        <FormField
                          name="tgCode"
                          label={t('pages.login.tgCode')}
                          rules={{ validate: rhfZodValidate(TgCodeSchema) }}
                        >
                          <Input
                            prefix={<KeyOutlined />}
                            autoComplete="one-time-code"
                            inputMode="numeric"
                            size="large"
                            placeholder={t('pages.login.tgCodePlaceholder')}
                          />
                        </FormField>
                      )}

                      {captcha && (
                        <FormField
                          name="captchaAnswer"
                          label={t('pages.login.captcha')}
                          rules={{ validate: rhfZodValidate(CaptchaAnswerSchema) }}
                        >
                          <Input
                            size="large"
                            autoComplete="off"
                            inputMode="numeric"
                            placeholder={t('pages.login.captchaPlaceholder')}
                            addonAfter={
                              <button
                                type="button"
                                className="captcha-refresh"
                                aria-label={t('pages.login.captchaRefresh')}
                                onClick={() => void loadCaptcha()}
                              >
                                <img src={captcha.image} alt="" />
                              </button>
                            }
                          />
                        </FormField>
                      )}

                      <Form.Item className="submit-row">
                        <Button
                          type="primary"
                          htmlType="submit"
                          loading={submitting}
                          size="large"
                          block
                        >
                          {t('login')}
                        </Button>
                      </Form.Item>
                      {passkeyLoginEnabled && (
                        <Form.Item className="submit-row">
                          <Button
                            block
                            size="large"
                            icon={<SafetyCertificateOutlined />}
                            loading={submitting}
                            onClick={() => void onPasskeyLogin()}
                          >
                            {t('pages.login.passkeyButton')}
                          </Button>
                        </Form.Item>
                      )}
                    </Form>
                  </FormProvider>
                  <SponsorSlot slot="login" variant="compact" className="login-sponsor" />
                </div>
              )}
            </div>
          </div>
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}
