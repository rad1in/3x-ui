import type { Meta, StoryObj } from '@storybook/react-vite';

import BrandMark from './BrandMark';

const LOGO =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYGD4DwABBAEAwS2OUAAAAABJRU5ErkJggg==';

const meta = {
  title: 'UI/BrandMark',
  component: BrandMark,
  tags: ['autodocs'],
  parameters: {
    docs: {
      description: {
        component:
          'Panel name and logo from the Branding settings (injected by the server as window.X_UI_BRAND), falling back to 3X-UI. The compact form shows only the logo, or initials when there is no logo, for the collapsed sidebar rail.',
      },
    },
  },
  argTypes: {
    compact: { description: 'Logo only, or initials without a logo.' },
    className: { description: 'Extra class for the wrapper.' },
  },
  afterEach: () => {
    delete window.X_UI_BRAND;
  },
} satisfies Meta<typeof BrandMark>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const CustomName: Story = {
  beforeEach: () => {
    window.X_UI_BRAND = { name: 'Acme VPN' };
  },
};

export const CompactInitials: Story = {
  args: { compact: true },
  beforeEach: () => {
    window.X_UI_BRAND = { name: 'Acme VPN' };
  },
};

export const WithLogo: Story = {
  beforeEach: () => {
    window.X_UI_BRAND = { name: 'Acme VPN', logo: LOGO };
  },
};
