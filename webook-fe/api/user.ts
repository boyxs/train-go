import type {
  EditProfileReq,
  LoginReq,
  Profile,
  RegisterReq,
  Result,
  SendCodeReq,
  SmsLoginReq,
  UserInfo,
} from '@/types';

import axios from './request';

// POST /user/register — 返回 Result
export function register(data: RegisterReq) {
  return axios.post<Result>('/user/register', data);
}

// POST /user/login — 返回 Result
export function login(data: LoginReq) {
  return axios.post<Result>('/user/login', data);
}

// POST /user/logout — 返回 Result
export function logout() {
  return axios.post<Result>('/user/logout');
}

// GET /user/profile — 直接返回 Profile 对象（无 Result 包装）
export function findProfile() {
  return axios.get<Profile>('/user/profile');
}

// POST /user/info — 他人主页取某用户公开信息（公开）
export function findUserInfo(id: number) {
  return axios.post<Result<UserInfo>>('/user/info', { id });
}

// POST /user/edit — 返回 Result<Profile>
export function updateProfile(data: EditProfileReq) {
  return axios.post<Result<Profile>>('/user/edit', data);
}

// POST /user/login_sms/code/send — 返回 Result
export function sendSmsCode(data: SendCodeReq) {
  return axios.post<Result>('/user/login_sms/code/send', data);
}

// POST /user/login_sms — 返回 Result
export function loginSms(data: SmsLoginReq) {
  return axios.post<Result>('/user/login_sms', data);
}

// GET /oauth2/wechat/authurl — 返回 Result<string>（微信授权 URL）
export function findWechatAuthUrl() {
  return axios.get<Result<string>>('/oauth2/wechat/authurl');
}
